// Copyright Tharsis Labs Ltd.(Evmos)
// SPDX-License-Identifier:ENCL-1.0(https://github.com/evmos/evmos/blob/main/LICENSE)
package erc20

import (
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
)

const (
	// TransferMethod defines the ABI method name for the ERC-20 transfer
	// transaction.
	TransferMethod = "transfer"
	// TransferFromMethod defines the ABI method name for the ERC-20 transferFrom
	// transaction.
	TransferFromMethod = "transferFrom"
)

// SendMsgURL defines the authorization type for MsgSend
var SendMsgURL = sdk.MsgTypeURL(&banktypes.MsgSend{})

// Transfer executes a direct transfer from the caller address to the
// destination address.
func (p Precompile) Transfer(
	ctx sdk.Context,
	contract *vm.Contract,
	stateDB vm.StateDB,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	from := contract.CallerAddress
	to, amount, err := ParseTransferArgs(args)
	if err != nil {
		return nil, err
	}

	return p.transfer(ctx, contract, stateDB, method, from, to, amount)
}

// TransferFrom executes a transfer on behalf of the specified from address in
// the call data to the destination address.
func (p Precompile) TransferFrom(
	ctx sdk.Context,
	contract *vm.Contract,
	stateDB vm.StateDB,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	from, to, amount, err := ParseTransferFromArgs(args)
	if err != nil {
		return nil, err
	}

	return p.transfer(ctx, contract, stateDB, method, from, to, amount)
}

// transfer is a common function that handles transfers for the ERC-20 Transfer
// and TransferFrom methods. It executes a bank Send message if the spender is
// the sender of the transfer, otherwise it executes an authorization.
func (p Precompile) transfer(
	ctx sdk.Context,
	contract *vm.Contract,
	stateDB vm.StateDB,
	method *abi.Method,
	from, to common.Address,
	amount *big.Int,
) (data []byte, err error) {
	coins := sdk.Coins{{Denom: p.tokenPair.Denom, Amount: sdk.NewIntFromBigInt(amount)}}

	msg := banktypes.NewMsgSend(from.Bytes(), to.Bytes(), coins)

	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	sender := sdk.AccAddress(from.Bytes())
	spender := sdk.AccAddress(contract.CallerAddress.Bytes()) // aka. grantee

	if sender.Equals(spender) {
		msgSrv := bankkeeper.NewMsgServerImpl(p.bankKeeper)
		_, err = msgSrv.Send(sdk.WrapSDKContext(ctx), msg)
	} else {
		_, err = p.AuthzKeeper.DispatchActions(ctx, spender, []sdk.Msg{msg})
	}

	if err != nil {
		// This should return an error to avoid the contract from being executed and an event being emitted
		return nil, err
	}

	if err := p.EmitTransferEvent(ctx, stateDB, from, to, amount); err != nil {
		return nil, err
	}

	return method.Outputs.Pack(true)
}