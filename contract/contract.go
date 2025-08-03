package contract

import (
	"math/rand"
)

/* This file contains the base contract implementation that overrides the basic 'transfer' functionality */

// PluginConfig: the configuration of the contract
var ContractConfig = &PluginConfig{
	Name:                  "send",
	Id:                    1,
	Version:               1,
	SupportedTransactions: []string{"send"},
}

// Contract() defines the smart contract that implements the extended logic of the nested chain
type Contract struct {
	FSMConfig *PluginFSMConfig // fsm configuration
	plugin    Plugin           // plugin connection
}

// Genesis() implements logic to import a json file to create the state at height 0 and export the state at any height
func (c *Contract) Genesis(_ *PluginGenesisRequest) *PluginGenesisResponse {
	return &PluginGenesisResponse{}
}

// BeginBlock() is code that is executed at the start of `applying` the block
func (c *Contract) BeginBlock(_ *PluginBeginRequest) *PluginBeginResponse {
	return &PluginBeginResponse{}
}

// CheckTx() is code that is executed to statelessly validate a transaction
func (c *Contract) CheckTx(request *PluginCheckRequest) *PluginCheckResponse {
	// validate fee
	resp, err := c.plugin.StateRead(c, &PluginStateReadRequest{
		Keys: []*PluginKeyRead{
			{QueryId: rand.Uint64(), Key: KeyForFeeParams()},
		}})
	if err == nil {
		err = resp.Error
	}
	// handle error
	if err != nil {
		return &PluginCheckResponse{Error: err}
	}
	// convert bytes into fee parameters
	minFees := new(FeeParams)
	if err = Unmarshal(resp.Results[0].Entries[0].Value, minFees); err != nil {
		return &PluginCheckResponse{Error: err}
	}
	// check for the minimum fee
	if request.Tx.Fee < minFees.SendFee {
		return &PluginCheckResponse{Error: ErrTxFeeBelowStateLimit()}
	}
	// get the message
	msg, err := FromAny(request.Tx.Msg)
	if err != nil {
		return &PluginCheckResponse{Error: err}
	}
	// handle the message
	switch x := msg.(type) {
	case *MessageSend:
		return c.CheckMessageSend(x)
	default:
		return &PluginCheckResponse{Error: ErrInvalidMessageCast()}
	}
}

// DeliverTx() is code that is executed to apply a transaction
func (c *Contract) DeliverTx(request *PluginDeliverRequest) *PluginDeliverResponse {
	// get the message
	msg, err := FromAny(request.Tx.Msg)
	if err != nil {
		return &PluginDeliverResponse{Error: err}
	}
	// handle the message
	switch x := msg.(type) {
	case *MessageSend:
		return c.DeliverMessageSend(x)
	default:
		return &PluginDeliverResponse{Error: ErrInvalidMessageCast()}
	}
}

// EndBlock() is code that is executed at the end of 'applying' a block
func (c *Contract) EndBlock(_ *PluginEndRequest) *PluginEndResponse {
	return &PluginEndResponse{}
}

// CheckMessageSend() statelessly validates a 'send' message
func (c *Contract) CheckMessageSend(msg *MessageSend) *PluginCheckResponse {
	// check sender address
	if len(msg.FromAddress) != 20 {
		return &PluginCheckResponse{Error: ErrInvalidAddress()}
	}
	// check recipient address
	if len(msg.FromAddress) != 20 {
		return &PluginCheckResponse{Error: ErrInvalidAddress()}
	}
	// check amount
	if msg.Amount == 0 {
		return &PluginCheckResponse{Error: ErrInvalidAmount()}
	}
	// return the authorized signers
	return &PluginCheckResponse{AuthorizedSigners: [][]byte{msg.FromAddress}}
}

// DeliverMessageSend() handles a 'send' message
func (c *Contract) DeliverMessageSend(msg *MessageSend) *PluginDeliverResponse {
	var (
		fromKey, toKey         []byte
		fromBytes, toBytes     []byte
		fromQueryId, toQueryId = rand.Uint64(), rand.Uint64()
		from, to               = new(Account), new(Account)
	)
	// calculate the from key and to key
	fromKey, toKey = KeyForAccount(msg.FromAddress), KeyForAccount(msg.ToAddress)
	// get the from and to account
	response, err := c.plugin.StateRead(c, &PluginStateReadRequest{
		Keys: []*PluginKeyRead{
			{QueryId: fromQueryId, Key: fromKey},
			{QueryId: toQueryId, Key: toKey},
		}})
	// check for internal error
	if err != nil {
		return &PluginDeliverResponse{Error: err}
	}
	// ensure no error fsm error
	if response.Error != nil {
		return &PluginDeliverResponse{Error: err}
	}
	// get the from bytes and to bytes
	for _, resp := range response.Results {
		if resp.QueryId == fromQueryId {
			fromBytes = resp.Entries[0].Value
		} else {
			toBytes = resp.Entries[0].Value
		}
	}
	// convert the bytes to account structures
	if err = Unmarshal(fromBytes, from); err != nil {
		return &PluginDeliverResponse{Error: err}
	}
	if err = Unmarshal(toBytes, to); err != nil {
		return &PluginDeliverResponse{Error: err}
	}
	// if the account amount is less than the amount to subtract; return insufficient funds
	if from.Amount < msg.Amount {
		return &PluginDeliverResponse{Error: ErrInsufficientFunds()}
	}
	// subtract from sender
	from.Amount -= msg.Amount
	// add to recipient
	to.Amount += msg.Amount
	// convert the accounts to bytes
	fromBytes, err = Marshal(from)
	if err != nil {
		return &PluginDeliverResponse{Error: err}
	}
	toBytes, err = Marshal(from)
	if err != nil {
		return &PluginDeliverResponse{Error: err}
	}
	// execute writes to the database
	var resp *PluginStateWriteResponse
	// if the from account is drained - delete the from account
	if from.Amount == 0 {
		resp, err = c.plugin.StateWrite(c, &PluginStateWriteRequest{
			Sets:    []*PluginSetOp{{Key: toKey, Value: toBytes}},
			Deletes: []*PluginDeleteOp{{Key: fromKey}},
		})
	} else {
		resp, err = c.plugin.StateWrite(c, &PluginStateWriteRequest{
			Sets: []*PluginSetOp{{Key: toKey, Value: toBytes}, {Key: fromKey, Value: fromBytes}},
		})
	}
	if err == nil {
		err = resp.Error
	}
	return &PluginDeliverResponse{Error: err}
}

var (
	accountPrefix = []byte{1} // store key prefix for accounts
	paramsPrefix  = []byte{7} // store key prefix for governance parameters
)

// KeyForAccount() returns the state database key for an account
func KeyForAccount(addr []byte) []byte {
	return JoinLenPrefix(accountPrefix, addr)
}

// KeyForFeeParams() returns the state database key for governance controlled 'fee parameters'
func KeyForFeeParams() []byte {
	return JoinLenPrefix(paramsPrefix, []byte("/f/"))
}
