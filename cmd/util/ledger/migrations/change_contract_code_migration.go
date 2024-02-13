package migrations

import (
	"context"
	"fmt"
	"strings"
	"sync"

	coreContracts "github.com/onflow/flow-core-contracts/lib/go/contracts"
	ftContracts "github.com/onflow/flow-ft/lib/go/contracts"
	nftContracts "github.com/onflow/flow-nft/lib/go/contracts"
	"github.com/rs/zerolog"

	sdk "github.com/onflow/flow-go-sdk"

	"github.com/onflow/cadence/runtime/common"

	evm "github.com/onflow/flow-go/fvm/evm/stdlib"
	"github.com/onflow/flow-go/fvm/systemcontracts"
	"github.com/onflow/flow-go/ledger"
	"github.com/onflow/flow-go/ledger/common/convert"
	"github.com/onflow/flow-go/model/flow"
)

type ChangeContractCodeMigration struct {
	log       zerolog.Logger
	mutex     sync.RWMutex
	contracts map[common.Address]map[flow.RegisterID]string
}

var _ AccountBasedMigration = (*ChangeContractCodeMigration)(nil)

func (d *ChangeContractCodeMigration) Close() error {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	if len(d.contracts) > 0 {
		var sb strings.Builder
		sb.WriteString("failed to find all contract registers that need to be changed:\n")
		for address, contracts := range d.contracts {
			_, _ = fmt.Fprintf(&sb, "- address: %s\n", address)
			for registerID := range contracts {
				_, _ = fmt.Fprintf(&sb, "  - %s\n", flow.RegisterIDContractName(registerID))
			}
		}
		return fmt.Errorf(sb.String())
	}

	return nil
}

func (d *ChangeContractCodeMigration) InitMigration(
	log zerolog.Logger,
	_ []*ledger.Payload,
	_ int,
) error {
	d.log = log.
		With().
		Str("migration", "ChangeContractCodeMigration").
		Logger()

	return nil
}

func (d *ChangeContractCodeMigration) MigrateAccount(
	_ context.Context,
	address common.Address,
	payloads []*ledger.Payload,
) ([]*ledger.Payload, error) {

	contracts, ok := (func() (map[flow.RegisterID]string, bool) {
		d.mutex.Lock()
		defer d.mutex.Unlock()

		contracts, ok := d.contracts[address]

		// remove address from set of addresses
		// to keep track of which addresses are left to change
		delete(d.contracts, address)

		return contracts, ok
	})()

	if !ok {
		// no contracts to change on this address
		return payloads, nil
	}

	for payloadIndex, payload := range payloads {
		key, err := payload.Key()
		if err != nil {
			return nil, err
		}

		registerID, err := convert.LedgerKeyToRegisterID(key)
		if err != nil {
			return nil, err
		}

		newContract, ok := contracts[registerID]
		if !ok {
			// not a contract register, or
			// not interested in this contract
			continue
		}

		// change contract code
		payloads[payloadIndex] = ledger.NewPayload(
			key,
			[]byte(newContract),
		)

		// TODO: maybe log diff between old and new

		// remove contract from list of contracts to change
		// to keep track of which contracts are left to change
		delete(contracts, registerID)
	}

	if len(contracts) > 0 {
		var sb strings.Builder
		_, _ = fmt.Fprintf(&sb, "failed to find all contract registers that need to be changed for address %s:\n", address)
		for registerID := range contracts {
			_, _ = fmt.Fprintf(&sb, "- %s\n", flow.RegisterIDContractName(registerID))
		}
		return nil, fmt.Errorf(sb.String())
	}

	return payloads, nil
}

func (d *ChangeContractCodeMigration) RegisterContractChange(
	address common.Address,
	contractName string,
	newContractCode string,
) (
	previousNewContractCode string,
) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.contracts == nil {
		d.contracts = map[common.Address]map[flow.RegisterID]string{}
	}

	if _, ok := d.contracts[address]; !ok {
		d.contracts[address] = map[flow.RegisterID]string{}
	}

	registerID := flow.ContractRegisterID(flow.ConvertAddress(address), contractName)

	previousNewContractCode = d.contracts[address][registerID]

	d.contracts[address][registerID] = newContractCode

	return
}

type SystemContractChange struct {
	Address         common.Address
	ContractName    string
	NewContractCode string
}

func NewSystemContractChange(
	systemContract systemcontracts.SystemContract,
	newContractCode []byte,
) SystemContractChange {
	return SystemContractChange{
		Address:         common.Address(systemContract.Address),
		ContractName:    systemContract.Name,
		NewContractCode: string(newContractCode),
	}
}

type EVMContractChange uint8

const (
	EVMContractChangeNone EVMContractChange = iota
	EVMContractChangeABIOnly
	EVMContractChangeFull
)

type SystemContractChangesOptions struct {
	EVM EVMContractChange
}

func SystemContractChanges(chainID flow.ChainID, options SystemContractChangesOptions) []SystemContractChange {
	systemContracts := systemcontracts.SystemContractsForChain(chainID)

	var stakingCollectionAddress, stakingProxyAddress common.Address

	switch chainID {
	case flow.Mainnet:
		stakingCollectionAddress = mustHexAddress("0x8d0e87b65159ae63")
		stakingProxyAddress = mustHexAddress("0x62430cf28c26d095")

	case flow.Testnet:
		stakingCollectionAddress = mustHexAddress("0x95e019a17d0e23d7")
		stakingProxyAddress = mustHexAddress("0x7aad92e5a0715d21")

	case flow.Emulator, flow.Localnet:
		stakingCollectionAddress = common.Address(systemContracts.FlowServiceAccount.Address)
		stakingProxyAddress = common.Address(systemContracts.FlowServiceAccount.Address)

	default:
		panic(fmt.Errorf("unsupported chain ID: %s", chainID))
	}

	lockedTokensAddress := stakingCollectionAddress
	fungibleTokenMetadataViewsAddress := common.Address(systemContracts.FungibleToken.Address)
	fungibleTokenSwitchboardAddress := common.Address(systemContracts.FungibleToken.Address)

	contractChanges := []SystemContractChange{
		// epoch related contracts
		NewSystemContractChange(
			systemContracts.Epoch,
			coreContracts.FlowEpoch(
				systemContracts.FungibleToken.Address.HexWithPrefix(),
				systemContracts.FlowToken.Address.HexWithPrefix(),
				systemContracts.IDTableStaking.Address.HexWithPrefix(),
				systemContracts.ClusterQC.Address.HexWithPrefix(),
				systemContracts.DKG.Address.HexWithPrefix(),
				systemContracts.FlowFees.Address.HexWithPrefix(),
			),
		),
		NewSystemContractChange(
			systemContracts.IDTableStaking,
			coreContracts.FlowIDTableStaking(
				systemContracts.FungibleToken.Address.HexWithPrefix(),
				systemContracts.FlowToken.Address.HexWithPrefix(),
				systemContracts.FlowFees.Address.HexWithPrefix(),
				systemContracts.FlowServiceAccount.Address.HexWithPrefix(),
				true,
			),
		),
		NewSystemContractChange(
			systemContracts.ClusterQC,
			coreContracts.FlowQC(),
		),
		NewSystemContractChange(
			systemContracts.DKG,
			coreContracts.FlowDKG(),
		),

		// service account related contracts
		NewSystemContractChange(
			systemContracts.FlowServiceAccount,
			coreContracts.FlowServiceAccount(
				systemContracts.FungibleToken.Address.HexWithPrefix(),
				systemContracts.FlowToken.Address.HexWithPrefix(),
				systemContracts.FlowFees.Address.HexWithPrefix(),
				systemContracts.FlowStorageFees.Address.HexWithPrefix(),
			),
		),
		NewSystemContractChange(
			systemContracts.NodeVersionBeacon,
			coreContracts.NodeVersionBeacon(),
		),
		NewSystemContractChange(
			systemContracts.RandomBeaconHistory,
			coreContracts.RandomBeaconHistory(),
		),
		NewSystemContractChange(
			systemContracts.FlowStorageFees,
			coreContracts.FlowStorageFees(
				systemContracts.FungibleToken.Address.HexWithPrefix(),
				systemContracts.FlowToken.Address.HexWithPrefix(),
			),
		),
		{
			Address:      stakingCollectionAddress,
			ContractName: "FlowStakingCollection",
			NewContractCode: string(coreContracts.FlowStakingCollection(
				systemContracts.FungibleToken.Address.HexWithPrefix(),
				systemContracts.FlowToken.Address.HexWithPrefix(),
				systemContracts.IDTableStaking.Address.HexWithPrefix(),
				stakingProxyAddress.HexWithPrefix(),
				lockedTokensAddress.HexWithPrefix(),
				systemContracts.FlowStorageFees.Address.HexWithPrefix(),
				systemContracts.ClusterQC.Address.HexWithPrefix(),
				systemContracts.DKG.Address.HexWithPrefix(),
				systemContracts.Epoch.Address.HexWithPrefix(),
			)),
		},
		{
			Address:         stakingProxyAddress,
			ContractName:    "StakingProxy",
			NewContractCode: string(coreContracts.FlowStakingProxy()),
		},
		{
			Address:      lockedTokensAddress,
			ContractName: "LockedTokens",
			NewContractCode: string(coreContracts.FlowLockedTokens(
				systemContracts.FungibleToken.Address.HexWithPrefix(),
				systemContracts.FlowToken.Address.HexWithPrefix(),
				systemContracts.IDTableStaking.Address.HexWithPrefix(),
				stakingProxyAddress.HexWithPrefix(),
				systemContracts.FlowStorageFees.Address.HexWithPrefix(),
			)),
		},

		// token related contracts
		NewSystemContractChange(
			systemContracts.FlowFees,
			coreContracts.FlowFees(
				systemContracts.FungibleToken.Address.HexWithPrefix(),
				systemContracts.FlowToken.Address.HexWithPrefix(),
				systemContracts.FlowStorageFees.Address.HexWithPrefix(),
			),
		),
		NewSystemContractChange(
			systemContracts.FlowToken,
			coreContracts.FlowToken(
				systemContracts.FungibleToken.Address.HexWithPrefix(),
				fungibleTokenMetadataViewsAddress.HexWithPrefix(),
				systemContracts.MetadataViews.Address.HexWithPrefix(),
				systemContracts.ViewResolver.Address.HexWithPrefix(),
			),
		),
		NewSystemContractChange(
			systemContracts.FungibleToken,
			ftContracts.FungibleToken(
				// Use `Hex()`, since this method adds the prefix.
				systemContracts.ViewResolver.Address.Hex(),
				systemContracts.FlowServiceAccount.Address.Hex(),
			),
		),
		{
			Address:      fungibleTokenMetadataViewsAddress,
			ContractName: "FungibleTokenMetadataViews",
			NewContractCode: string(ftContracts.FungibleTokenMetadataViews(
				// Use `Hex()`, since this method adds the prefix.
				systemContracts.FungibleToken.Address.Hex(),
				systemContracts.MetadataViews.Address.Hex(),
				systemContracts.ViewResolver.Address.Hex(),
			)),
		},

		// NFT related contracts
		NewSystemContractChange(
			systemContracts.NonFungibleToken,
			nftContracts.NonFungibleToken(
				sdk.Address(systemContracts.ViewResolver.Address),
			),
		),
		NewSystemContractChange(
			systemContracts.MetadataViews,
			nftContracts.MetadataViews(
				sdk.Address(systemContracts.FungibleToken.Address),
				sdk.Address(systemContracts.NonFungibleToken.Address),
				sdk.Address(systemContracts.ViewResolver.Address),
			),
		),
		NewSystemContractChange(
			systemContracts.ViewResolver,
			nftContracts.ViewResolver(),
		),
	}

	switch chainID {
	case flow.Emulator, flow.Localnet:
		// skip

	default:
		contractChanges = append(
			contractChanges,
			SystemContractChange{
				Address:      fungibleTokenSwitchboardAddress,
				ContractName: "FungibleTokenSwitchboard",
				NewContractCode: string(ftContracts.FungibleTokenSwitchboard(
					systemContracts.FungibleToken.Address.HexWithPrefix(),
				)),
			},
		)
	}

	// EVM related contracts
	switch options.EVM {
	case EVMContractChangeNone:
		// do nothing
	case EVMContractChangeABIOnly, EVMContractChangeFull:
		abiOnly := options.EVM == EVMContractChangeABIOnly
		contractChanges = append(
			contractChanges,
			NewSystemContractChange(
				systemContracts.EVMContract,
				evm.ContractCode(
					systemContracts.FlowToken.Address,
					abiOnly,
				),
			),
		)
	default:
		panic(fmt.Errorf("unsupported EVM contract change option: %d", options.EVM))
	}

	return contractChanges
}

func mustHexAddress(hexAddress string) common.Address {
	address, err := common.HexToAddress(hexAddress)
	if err != nil {
		panic(err)
	}
	return address
}

func NewSystemContactsMigration(
	chainID flow.ChainID,
	options SystemContractChangesOptions,
) *ChangeContractCodeMigration {
	migration := &ChangeContractCodeMigration{}
	for _, change := range SystemContractChanges(chainID, options) {
		migration.RegisterContractChange(
			change.Address,
			change.ContractName,
			change.NewContractCode,
		)
	}
	return migration
}
