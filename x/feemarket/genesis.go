package feemarket

import (
	"fmt"
	"os"
	"path"

	sdk "github.com/cosmos/cosmos-sdk/types"
	abci "github.com/tendermint/tendermint/abci/types"

	"github.com/tharsis/ethermint/x/feemarket/keeper"
	"github.com/tharsis/ethermint/x/feemarket/types"
)

// InitGenesis initializes genesis state based on exported genesis
func InitGenesis(
	ctx sdk.Context,
	k keeper.Keeper,
	data types.GenesisState,
) []abci.ValidatorUpdate {
	k.SetParams(ctx, data.Params)
	k.SetBaseFee(ctx, data.BaseFee.BigInt())
	k.SetBlockGasUsed(ctx, data.BlockGas)

	return []abci.ValidatorUpdate{}
}

// ExportGenesis exports genesis state of the fee market module
func ExportGenesis(ctx sdk.Context, k keeper.Keeper) *types.GenesisState {
	baseFee := sdk.ZeroInt()
	baseFeeInt := k.GetBaseFee(ctx)
	if baseFeeInt != nil {
		baseFee = sdk.NewIntFromBigInt(baseFeeInt)
	}

	return &types.GenesisState{
		Params:   k.GetParams(ctx),
		BaseFee:  baseFee,
		BlockGas: k.GetBlockGasUsed(ctx),
	}
}

func ExportGenesisTo(ctx sdk.Context, k keeper.Keeper, exportPath string) error {
	if err := os.MkdirAll(exportPath, 0755); err != nil {
		return err
	}

	filePath := path.Join(exportPath, fmt.Sprintf("%s%d", types.ModuleName, 0))
	f, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return err
	}
	defer f.Close()

	gs := ExportGenesis(ctx, k)
	encoded, err := gs.Marshal()
	if err != nil {
		return err
	}
	_, err = f.Write(encoded)
	return err
}
