package evm

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"runtime/debug"

	"github.com/cosmos/cosmos-sdk/codec"
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

	fmt.Printf("emptycodehash: %s\n", common.BytesToHash(crypto.Keccak256(nil)).String())

	procDir, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	fp := path.Join(procDir, "accountExport.log")
	f, err := os.Create(fp)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	for i, account := range data.Accounts {
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
		//!bytes.Equal(codeHash.Bytes(), emptycodehash) &&
		if !bytes.Equal(ethAcct.GetCodeHash().Bytes(), codeHash.Bytes()) {
			s := fmt.Sprintf("code hash mismatch acc: %s\n, index:%d/%d, codeHash: %v, ethAcctHash: %v, accType: %d, ethaddr: %s\n", account, i, len(data.Accounts), codeHash, ethAcct.GetCodeHash(), ethAcct.Type(), ethAcct.EthAddress().Hex())
			f.WriteString(s)
			//panic("code don't match codeHash")
			continue
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

func Recover() {
	if r := recover(); r != nil {
		if _, ok := r.(error); ok {
			fmt.Printf("dump err: %s\n", debug.Stack())
		}
	}
}

func InitGenesisFrom(ctx sdk.Context,
	cdc codec.JSONCodec,
	k *keeper.Keeper,
	ak types.AccountKeeper,
	importPath string,
) ([]abci.ValidatorUpdate, error) {
	fp := path.Join(importPath, fmt.Sprintf("genesis_%s.bin", types.ModuleName))
	f, err := os.OpenFile(fp, os.O_RDONLY, 0666)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}

	bz := make([]byte, fi.Size())
	offset := 0
	for {
		n, err := f.ReadAt(bz[offset:], int64(offset))
		if err != nil {
			return nil, err
		}

		offset += n
		fmt.Printf("%d/%d\n", offset, fi.Size())

		if n == 0 || offset == len(bz) {
			break
		}
	}

	var gs types.GenesisState
	cdc.MustUnmarshalJSON(bz, &gs)
	return InitGenesis(ctx, k, ak, gs), nil
}

func ExportGenesisTo(ctx sdk.Context, cdc codec.JSONCodec, k *keeper.Keeper, ak types.AccountKeeper, exportPath string) error {
	defer Recover()

	if err := os.MkdirAll(exportPath, 0755); err != nil {
		return err
	}

	fp := path.Join(exportPath, fmt.Sprintf("genesis_%s.bin", types.ModuleName))
	f, err := os.Create(fp)
	if err != nil {
		return err
	}
	defer f.Close()

	gs := ExportGenesis(ctx, k, ak)
	bz := cdc.MustMarshalJSON(gs)
	if _, err := f.Write(bz); err != nil {
		return err
	}

	return nil
}
