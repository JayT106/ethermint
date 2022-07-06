package evm

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path"

	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	abci "github.com/tendermint/tendermint/abci/types"

	ethermint "github.com/tharsis/ethermint/types"
	"github.com/tharsis/ethermint/x/evm/keeper"
	"github.com/tharsis/ethermint/x/evm/types"
)

// InitGenesis initializes genesis state based on exported genesis
func InitGenesis(
	ctx sdk.Context,
	k *keeper.Keeper,
	accountKeeper types.AccountKeeper,
	data types.GenesisState,
) []abci.ValidatorUpdate {
	k.WithChainID(ctx)

	k.SetParams(ctx, data.Params)

	// ensure evm module account is set
	if addr := accountKeeper.GetModuleAddress(types.ModuleName); addr == nil {
		panic("the EVM module account has not been set")
	}

	for _, account := range data.Accounts {
		address := common.HexToAddress(account.Address)
		accAddress := sdk.AccAddress(address.Bytes())
		// check that the EVM balance the matches the account balance
		acc := accountKeeper.GetAccount(ctx, accAddress)
		if acc == nil {
			panic(fmt.Errorf("account not found for address %s", account.Address))
		}

		ethAcct, ok := acc.(ethermint.EthAccountI)
		if !ok {
			panic(
				fmt.Errorf("account %s must be an EthAccount interface, got %T",
					account.Address, acc,
				),
			)
		}

		code := common.Hex2Bytes(account.Code)
		codeHash := crypto.Keccak256Hash(code)
		if !bytes.Equal(ethAcct.GetCodeHash().Bytes(), codeHash.Bytes()) {
			panic("code don't match codeHash")
		}

		k.SetCode(ctx, codeHash.Bytes(), code)

		for _, storage := range account.Storage {
			k.SetState(ctx, address, common.HexToHash(storage.Key), common.HexToHash(storage.Value).Bytes())
		}
	}

	return []abci.ValidatorUpdate{}
}

// ExportGenesis exports genesis state of the EVM module
func ExportGenesis(ctx sdk.Context, k *keeper.Keeper, ak types.AccountKeeper) *types.GenesisState {
	var ethGenAccounts []types.GenesisAccount
	ak.IterateAccounts(ctx, func(account authtypes.AccountI) bool {
		ethAccount, ok := account.(ethermint.EthAccountI)
		if !ok {
			// ignore non EthAccounts
			return false
		}

		addr := ethAccount.EthAddress()

		storage := k.GetAccountStorage(ctx, addr)

		genAccount := types.GenesisAccount{
			Address: addr.String(),
			Code:    common.Bytes2Hex(k.GetCode(ctx, ethAccount.GetCodeHash())),
			Storage: storage,
		}

		ethGenAccounts = append(ethGenAccounts, genAccount)
		return false
	})

	return &types.GenesisState{
		Accounts: ethGenAccounts,
		Params:   k.GetParams(ctx),
	}
}

func ExportGenesisTo(ctx sdk.Context, k *keeper.Keeper, ak types.AccountKeeper, exportPath string) error {
	if err := os.MkdirAll(exportPath, 0755); err != nil {
		return err
	}

	var fileIndex = 0
	fn := fmt.Sprintf("genesis%d", fileIndex)
	filePath := path.Join(exportPath, fn)
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	// write the params
	param := k.GetParams(ctx)
	encodedParam, err := param.Marshal()
	if err != nil {
		return err
	}

	fs := 0
	offset := 0
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, uint32(len(encodedParam)))
	n, err := f.Write(b)
	if err != nil {
		return err
	}
	fs += n

	n, err = f.Write(encodedParam)
	if err != nil {
		return err
	}
	fs += n
	offset = fs
	counts := 0
	// leaving space for writing toal account numbers
	b = make([]byte, 8)
	binary.LittleEndian.PutUint64(b, 0)
	n, err = f.Write(b)
	if err != nil {
		return err
	}
	fs += n

	// write the account info into marshal proto message.
	ctxDone := false
	var e = error(nil)

	ak.IterateAccounts(ctx, func(account authtypes.AccountI) bool {
		select {
		case <-ctx.Context().Done():
			ctxDone = true
			return true
		default:
			ethAccount, ok := account.(ethermint.EthAccountI)
			if !ok {
				// ignore non EthAccounts
				return false
			}

			addr := ethAccount.EthAddress()
			storage := k.GetAccountStorage(ctx, addr)

			genAccount := types.GenesisAccount{
				Address: addr.String(),
				Code:    common.Bytes2Hex(k.GetCode(ctx, ethAccount.GetCodeHash())),
				Storage: storage,
			}

			bz, err := genAccount.Marshal()
			if err != nil {
				e = fmt.Errorf("genesus account marshal err: %s", err)
				return true
			}

			b := make([]byte, 4)
			binary.LittleEndian.PutUint32(b, uint32(len(bz)))
			n, err = f.Write(b)
			if err != nil {
				e = err
				return true
			}
			fs += n

			n, err = f.Write(bz)
			if err != nil {
				e = err
				return true
			}
			fs += n

			// we limited the file size to 100M
			if fs > 100000000 {
				err := f.Close()
				if err != nil {
					e = err
					return true
				}

				fileIndex++
				f, err = os.Create(filePath)
				if err != nil {
					e = err
					return true
				}

				fs = 0
			}

			counts++
			return false
		}
	})

	if ctxDone {
		return errors.New("genesus export terminated")
	}

	if e != nil {
		return e
	}

	// close the current file and reopen the first file and update
	// the account numbers in the file
	err = f.Close()
	if err != nil {
		return err
	}

	fileIndex = 0
	f, err = os.OpenFile(filePath, os.O_RDWR, 0644)
	if err != nil {
		return err
	}

	b = make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(counts))
	_, err = f.WriteAt(b, int64(offset))
	if err != nil {
		return err
	}

	return nil
}
