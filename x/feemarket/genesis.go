package feemarket

import (
	"fmt"
	"os"
	"path"

	"github.com/cosmos/cosmos-sdk/codec"
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

func InitGenesisFrom(ctx sdk.Context, cdc codec.JSONCodec, k keeper.Keeper, importPath string) ([]abci.ValidatorUpdate, error) {
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
	if _, err := f.Read(bz); err != nil {
		return nil, err
	}

	var gs types.GenesisState
	cdc.MustUnmarshalJSON(bz, &gs)
	return InitGenesis(ctx, k, gs), nil
}

func ExportGenesisTo(ctx sdk.Context, cdc codec.JSONCodec, k keeper.Keeper, exportPath string) error {
	if err := os.MkdirAll(exportPath, 0755); err != nil {
		return err
	}

	fp := path.Join(exportPath, fmt.Sprintf("genesis_%s.bin", types.ModuleName))
	f, err := os.Create(fp)
	if err != nil {
		return err
	}
	defer f.Close()

	gs := ExportGenesis(ctx, k)
	bz := cdc.MustMarshalJSON(gs)
	if _, err := f.Write(bz); err != nil {
		return err
	}

	return nil
}
