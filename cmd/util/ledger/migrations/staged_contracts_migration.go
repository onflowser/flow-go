package migrations

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/onflow/cadence/runtime"
	"github.com/onflow/cadence/runtime/common"
	"github.com/onflow/cadence/runtime/interpreter"
	"github.com/onflow/cadence/runtime/old_parser"
	"github.com/onflow/cadence/runtime/pretty"
	"github.com/onflow/cadence/runtime/sema"
	"github.com/onflow/cadence/runtime/stdlib"
	"github.com/rs/zerolog"

	"github.com/onflow/flow-go/cmd/util/ledger/reporters"
	"github.com/onflow/flow-go/cmd/util/ledger/util/snapshot"
	"github.com/onflow/flow-go/ledger"
	"github.com/onflow/flow-go/ledger/common/convert"
	"github.com/onflow/flow-go/model/flow"

	coreContracts "github.com/onflow/flow-core-contracts/lib/go/contracts"
)

type StagedContractsMigration struct {
	name                           string
	chainID                        flow.ChainID
	log                            zerolog.Logger
	mutex                          sync.RWMutex
	stagedContracts                map[common.Address]map[flow.RegisterID]Contract
	contractsByLocation            map[common.Location][]byte
	enableUpdateValidation         bool
	userDefinedTypeChangeCheckFunc func(oldTypeID common.TypeID, newTypeID common.TypeID) (checked bool, valid bool)
	elaborations                   map[common.Location]*sema.Elaboration
	contractAdditionHandler        stdlib.AccountContractAdditionHandler
	contractNamesProvider          stdlib.AccountContractNamesProvider
	reporter                       reporters.ReportWriter
	verboseErrorOutput             bool
	nWorkers                       int
}

type StagedContract struct {
	Contract
	Address common.Address
}

type Contract struct {
	Name string
	Code []byte
}

var _ AccountBasedMigration = &StagedContractsMigration{}

type StagedContractsMigrationOptions struct {
	ChainID            flow.ChainID
	VerboseErrorOutput bool
}

func NewStagedContractsMigration(
	name string,
	reporterName string,
	log zerolog.Logger,
	rwf reporters.ReportWriterFactory,
	options StagedContractsMigrationOptions,
) *StagedContractsMigration {
	return &StagedContractsMigration{
		name:                name,
		log:                 log,
		chainID:             options.ChainID,
		stagedContracts:     map[common.Address]map[flow.RegisterID]Contract{},
		contractsByLocation: map[common.Location][]byte{},
		reporter:            rwf.ReportWriter(reporterName),
		verboseErrorOutput:  options.VerboseErrorOutput,
	}
}

func (m *StagedContractsMigration) WithContractUpdateValidation() *StagedContractsMigration {
	m.enableUpdateValidation = true
	m.userDefinedTypeChangeCheckFunc = NewUserDefinedTypeChangeCheckerFunc(m.chainID)
	return m
}

// WithStagedContractUpdates prepares the contract updates as a map for easy lookup.
func (m *StagedContractsMigration) WithStagedContractUpdates(stagedContracts []StagedContract) *StagedContractsMigration {
	m.registerContractUpdates(stagedContracts)
	m.log.Info().
		Msgf("total of %d staged contracts are provided externally", len(m.contractsByLocation))

	return m
}

func (m *StagedContractsMigration) Close() error {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Close the report writer so it flushes to file.
	m.reporter.Close()

	if len(m.stagedContracts) > 0 {
		dict := zerolog.Dict()
		for address, contracts := range m.stagedContracts {
			arr := zerolog.Arr()
			for registerID := range contracts {
				arr = arr.Str(flow.RegisterIDContractName(registerID))
			}
			dict = dict.Array(
				address.HexWithPrefix(),
				arr,
			)
		}
		m.log.Error().
			Dict("contracts", dict).
			Msg("failed to find all contract registers that need to be changed")
	}

	return nil
}

func (m *StagedContractsMigration) InitMigration(
	log zerolog.Logger,
	allPayloads []*ledger.Payload,
	nWorkers int,
) error {
	m.log = log.
		With().
		Str("migration", m.name).
		Logger()

	err := m.collectAndRegisterStagedContractsFromPayloads(allPayloads)
	if err != nil {
		return err
	}

	// Manually register burner contract
	burnerLocation := common.AddressLocation{
		Name:    "Burner",
		Address: common.Address(BurnerAddressForChain(m.chainID)),
	}
	m.contractsByLocation[burnerLocation] = coreContracts.Burner()

	// Initialize elaborations, ContractAdditionHandler and ContractNamesProvider.
	// These needs to be initialized using **ALL** payloads, not just the payloads of the account.

	elaborations := map[common.Location]*sema.Elaboration{}

	config := MigratorRuntimeConfig{
		GetCode: func(location common.AddressLocation) ([]byte, error) {
			return m.contractsByLocation[location], nil
		},
		GetOrLoadProgramListener: func(location runtime.Location, program *interpreter.Program, err error) {
			if err == nil {
				elaborations[location] = program.Elaboration
			}
		},
	}

	// Pass empty address. We are only interested in the created `env` object.
	mr, err := NewMigratorRuntime(
		log,
		allPayloads,
		m.chainID,
		config,
		snapshot.SmallChangeSetSnapshot,
		nWorkers,
	)
	if err != nil {
		return err
	}

	m.elaborations = elaborations
	m.contractAdditionHandler = mr.ContractAdditionHandler
	m.contractNamesProvider = mr.ContractNamesProvider
	m.nWorkers = nWorkers

	return nil
}

// collectAndRegisterStagedContractsFromPayloads scan through the payloads and collects the contracts
// staged through the `MigrationContractStaging` contract.
func (m *StagedContractsMigration) collectAndRegisterStagedContractsFromPayloads(allPayloads []*ledger.Payload) error {

	// If the contracts are already passed as an input to the migration
	// then no need to scan the storage.
	if len(m.contractsByLocation) > 0 {
		return nil
	}

	var stagingAccount string

	switch m.chainID {
	case flow.Testnet:
		stagingAccount = "0x2ceae959ed1a7e7a"
	case flow.Mainnet:
		stagingAccount = "0x56100d46aa9b0212"
	default:
		// For other networks such as emulator etc. no need to scan for staged contracts.
		m.log.Warn().Msgf("staged contracts are not collected for %s state", m.chainID)
		return nil
	}

	stagingAccountAddress := common.Address(flow.HexToAddress(stagingAccount))

	// Filter-in only the payloads belong to the staging account.
	stagingAccountPayloads := make([]*ledger.Payload, 0)
	for _, payload := range allPayloads {
		key, err := payload.Key()
		if err != nil {
			return err
		}

		address := flow.BytesToAddress(key.KeyParts[0].Value)

		if common.Address(address) == stagingAccountAddress {
			stagingAccountPayloads = append(stagingAccountPayloads, payload)
		}
	}

	m.log.Info().
		Msgf("found %d payloads in account %s", len(stagingAccountPayloads), stagingAccount)

	mr, err := NewMigratorRuntime(
		m.log,
		stagingAccountPayloads,
		m.chainID,
		MigratorRuntimeConfig{},
		snapshot.SmallChangeSetSnapshot,
		m.nWorkers,
	)
	if err != nil {
		return err
	}

	inter := mr.Interpreter
	locationRange := interpreter.EmptyLocationRange

	storageMap := mr.Storage.GetStorageMap(
		stagingAccountAddress,
		common.PathDomainStorage.Identifier(),
		false,
	)
	if storageMap == nil {
		m.log.Error().
			Msgf("failed to get staged contracts from account %s", stagingAccount)
		return nil
	}

	iterator := storageMap.Iterator(inter)

	stagedContractCapsuleStaticType := interpreter.NewCompositeStaticTypeComputeTypeID(
		nil,
		common.AddressLocation{
			Name:    "MigrationContractStaging",
			Address: stagingAccountAddress,
		},
		"MigrationContractStaging.Capsule",
	)

	stagedContracts := make([]StagedContract, 0)

	for key, value := iterator.Next(); key != nil; key, value = iterator.Next() {
		stringAtreeValue, ok := key.(interpreter.StringAtreeValue)
		if !ok {
			continue
		}

		storagePath := string(stringAtreeValue)

		// Only consider paths that starts with "MigrationContractStagingCapsule_".
		if !strings.HasPrefix(storagePath, "MigrationContractStagingCapsule_") {
			continue
		}

		staticType := value.StaticType(inter)
		if !staticType.Equal(stagedContractCapsuleStaticType) {
			// This shouldn't occur, but technically possible.
			// e.g: accidentally storing other values under the same storage path pattern.
			// So skip such values. We are not interested in those.
			m.log.Debug().
				Msgf("found a value with an unexpected type `%s`", staticType)
			continue
		}

		stagedContract, err := m.getStagedContractFromValue(value, inter, locationRange)
		if err != nil {
			return err
		}

		stagedContracts = append(stagedContracts, stagedContract)
	}

	m.log.Info().
		Msgf("found %d staged contracts from payloads", len(stagedContracts))

	m.registerContractUpdates(stagedContracts)
	m.log.Info().
		Msgf("total of %d unique contracts are staged for all accounts", len(m.contractsByLocation))

	return nil
}

func (m *StagedContractsMigration) getStagedContractFromValue(
	value interpreter.Value,
	inter *interpreter.Interpreter,
	locationRange interpreter.LocationRange,
) (StagedContract, error) {

	stagedContractCapsule, ok := value.(*interpreter.CompositeValue)
	if !ok {
		return StagedContract{},
			fmt.Errorf("unexpected value of type %T", value)
	}

	// The stored value should take the form of:
	//
	// resource Capsule {
	//     let update: ContractUpdate
	// }
	//
	// struct ContractUpdate {
	//     let address: Address
	//     let name: String
	//     var code: String
	//     var lastUpdated: UFix64
	// }

	updateField := stagedContractCapsule.GetField(inter, locationRange, "update")
	contractUpdate, ok := updateField.(*interpreter.CompositeValue)
	if !ok {
		return StagedContract{},
			fmt.Errorf("unexpected value: expected `CompositeValue`, found `%T`", updateField)
	}

	addressField := contractUpdate.GetField(inter, locationRange, "address")
	address, ok := addressField.(interpreter.AddressValue)
	if !ok {
		return StagedContract{},
			fmt.Errorf("unexpected value: expected `AddressValue`, found `%T`", addressField)
	}

	nameField := contractUpdate.GetField(inter, locationRange, "name")
	name, ok := nameField.(*interpreter.StringValue)
	if !ok {
		return StagedContract{},
			fmt.Errorf("unexpected value: expected `StringValue`, found `%T`", nameField)
	}

	codeField := contractUpdate.GetField(inter, locationRange, "code")
	code, ok := codeField.(*interpreter.StringValue)
	if !ok {
		return StagedContract{},
			fmt.Errorf("unexpected value: expected `StringValue`, found `%T`", codeField)
	}

	return StagedContract{
		Contract: Contract{
			Name: name.Str,
			Code: []byte(code.Str),
		},
		Address: common.Address(address),
	}, nil
}

// registerContractUpdates prepares the contract updates as a map for easy lookup.
func (m *StagedContractsMigration) registerContractUpdates(stagedContracts []StagedContract) {
	for _, contractChange := range stagedContracts {
		m.registerContractChange(contractChange)
	}
}

func (m *StagedContractsMigration) registerContractChange(change StagedContract) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	address := change.Address

	chain := m.chainID.Chain()

	if _, err := chain.IndexFromAddress(flow.Address(address)); err != nil {
		m.log.Error().Msgf(
			"invalid contract update: invalid address for chain %s: %s (%s)",
			m.chainID,
			address.HexWithPrefix(),
			change.Name,
		)
	}

	if _, ok := m.stagedContracts[address]; !ok {
		m.stagedContracts[address] = map[flow.RegisterID]Contract{}
	}

	name := change.Name

	registerID := flow.ContractRegisterID(flow.ConvertAddress(address), name)

	_, exist := m.stagedContracts[address][registerID]
	if exist {
		// Staged multiple updates for the same contract.
		// Overwrite the previous update.
		m.log.Warn().Msgf(
			"existing staged update found for contract %s.%s. Previous update will be overwritten.",
			address.HexWithPrefix(),
			name,
		)
	}

	m.stagedContracts[address][registerID] = change.Contract

	location := common.AddressLocation{
		Name:    name,
		Address: address,
	}
	m.contractsByLocation[location] = change.Code
}

func (m *StagedContractsMigration) contractUpdatesForAccount(
	address common.Address,
) (map[flow.RegisterID]Contract, bool) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	contracts, ok := m.stagedContracts[address]

	// remove address from set of addresses
	// to keep track of which addresses are left to change
	delete(m.stagedContracts, address)

	return contracts, ok
}

func (m *StagedContractsMigration) MigrateAccount(
	_ context.Context,
	address common.Address,
	oldPayloads []*ledger.Payload,
) ([]*ledger.Payload, error) {

	checkPayloadsOwnership(oldPayloads, address, m.log)

	contractUpdates, ok := m.contractUpdatesForAccount(address)
	if !ok {
		// no contracts to change on this address
		return oldPayloads, nil
	}

	for payloadIndex, payload := range oldPayloads {
		key, err := payload.Key()
		if err != nil {
			return nil, err
		}

		registerID, err := convert.LedgerKeyToRegisterID(key)
		if err != nil {
			return nil, err
		}

		updatedContract, ok := contractUpdates[registerID]
		if !ok {
			// not a contract register, or
			// not interested in this contract
			continue
		}

		name := updatedContract.Name
		newCode := updatedContract.Code
		oldCode := payload.Value()

		if m.enableUpdateValidation {
			err = m.checkContractUpdateValidity(
				address,
				name,
				newCode,
				oldCode,
			)
		}

		if err != nil {
			var builder strings.Builder
			errorPrinter := pretty.NewErrorPrettyPrinter(&builder, false)

			location := common.AddressLocation{
				Name:    name,
				Address: address,
			}
			printErr := errorPrinter.PrettyPrintError(err, location, m.contractsByLocation)

			var errorDetails string
			if printErr == nil {
				errorDetails = builder.String()
			} else {
				errorDetails = err.Error()
			}

			if m.verboseErrorOutput {
				m.log.Error().
					Msgf(
						"failed to update contract %s in account %s: %s",
						name,
						address.HexWithPrefix(),
						errorDetails,
					)
			}

			m.reporter.Write(contractUpdateFailureEntry{
				AccountAddress: address,
				ContractName:   name,
				Error:          errorDetails,
			})
		} else {
			// change contract code
			oldPayloads[payloadIndex] = ledger.NewPayload(
				key,
				newCode,
			)

			m.reporter.Write(contractUpdateEntry{
				AccountAddress: address,
				ContractName:   name,
			})
		}

		// remove contract from list of contracts to change
		// to keep track of which contracts are left to change
		delete(contractUpdates, registerID)
	}

	if len(contractUpdates) > 0 {
		arr := zerolog.Arr()
		for registerID := range contractUpdates {
			arr = arr.Str(flow.RegisterIDContractName(registerID))
		}
		m.log.Error().
			Array("contracts", arr).
			Str("address", address.HexWithPrefix()).
			Msg("failed to find all contract registers that need to be changed for address")
	}

	return oldPayloads, nil
}

func (m *StagedContractsMigration) checkContractUpdateValidity(
	address common.Address,
	contractName string,
	newCode []byte,
	oldCode ledger.Value,
) error {
	// Parsing and checking of programs has to be done synchronously.
	m.mutex.Lock()
	defer m.mutex.Unlock()

	location := common.AddressLocation{
		Name:    contractName,
		Address: address,
	}

	// NOTE: do NOT use the program obtained from the host environment, as the current program.
	// Always re-parse and re-check the new program.
	// NOTE: *DO NOT* store the program – the new or updated program
	// should not be effective during the execution
	const getAndSetProgram = false

	newProgram, err := m.contractAdditionHandler.ParseAndCheckProgram(newCode, location, getAndSetProgram)
	if err != nil {
		return err
	}

	oldProgram, err := old_parser.ParseProgram(nil, oldCode, old_parser.Config{})
	if err != nil {
		return err
	}

	validator := stdlib.NewCadenceV042ToV1ContractUpdateValidator(
		location,
		contractName,
		m.contractNamesProvider,
		oldProgram,
		newProgram,
		m.elaborations,
	)

	validator.WithUserDefinedTypeChangeChecker(
		m.userDefinedTypeChangeCheckFunc,
	)

	return validator.Validate()
}

func StagedContractsFromCSV(path string) ([]StagedContract, error) {
	if path == "" {
		return nil, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	defer file.Close()

	reader := csv.NewReader(file)

	// Expect 3 fields: address, name, code
	reader.FieldsPerRecord = 3

	var contracts []StagedContract

	for {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, err
		}

		addressHex := rec[0]
		name := rec[1]
		code := rec[2]

		address, err := common.HexToAddress(addressHex)
		if err != nil {
			return nil, err
		}

		contracts = append(contracts, StagedContract{
			Contract: Contract{
				Name: name,
				Code: []byte(code),
			},
			Address: address,
		})
	}

	return contracts, nil
}

func NewUserDefinedTypeChangeCheckerFunc(
	chainID flow.ChainID,
) func(oldTypeID common.TypeID, newTypeID common.TypeID) (checked, valid bool) {

	typeChangeRules := map[common.TypeID]common.TypeID{}

	compositeTypeRules := NewCompositeTypeConversionRules(chainID)
	for typeID, newStaticType := range compositeTypeRules {
		typeChangeRules[typeID] = newStaticType.ID()
	}

	interfaceTypeRules := NewInterfaceTypeConversionRules(chainID)
	for typeID, newStaticType := range interfaceTypeRules {
		typeChangeRules[typeID] = newStaticType.ID()
	}

	return func(oldTypeID common.TypeID, newTypeID common.TypeID) (checked, valid bool) {
		expectedNewTypeID, found := typeChangeRules[oldTypeID]
		if found {
			return true, expectedNewTypeID == newTypeID
		}
		return false, false
	}
}

// contractUpdateFailureEntry

type contractUpdateEntry struct {
	AccountAddress common.Address
	ContractName   string
}

var _ json.Marshaler = contractUpdateEntry{}

func (e contractUpdateEntry) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Kind           string `json:"kind"`
		AccountAddress string `json:"account_address"`
		ContractName   string `json:"contract_name"`
	}{
		Kind:           "contract-update-success",
		AccountAddress: e.AccountAddress.HexWithPrefix(),
		ContractName:   e.ContractName,
	})
}

// contractUpdateFailureEntry

type contractUpdateFailureEntry struct {
	AccountAddress common.Address
	ContractName   string
	Error          string
}

var _ json.Marshaler = contractUpdateFailureEntry{}

func (e contractUpdateFailureEntry) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Kind           string `json:"kind"`
		AccountAddress string `json:"account_address"`
		ContractName   string `json:"contract_name"`
		Error          string `json:"error"`
	}{
		Kind:           "contract-update-failure",
		AccountAddress: e.AccountAddress.HexWithPrefix(),
		ContractName:   e.ContractName,
		Error:          e.Error,
	})
}
