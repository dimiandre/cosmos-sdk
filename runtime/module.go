package runtime

import (
	"fmt"

	"cosmossdk.io/depinject"
	abci "github.com/tendermint/tendermint/abci/types"
	"cosmossdk.io/core/store"

	abci "github.com/cometbft/cometbft/abci/types"

	runtimev1alpha1 "cosmossdk.io/api/cosmos/app/runtime/v1alpha1"
	appv1alpha1 "cosmossdk.io/api/cosmos/app/v1alpha1"
	"cosmossdk.io/core/intermodule"

	"cosmossdk.io/core/appmodule"
	"cosmossdk.io/depinject"

	storetypes "cosmossdk.io/store/types"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/std"
	"github.com/cosmos/cosmos-sdk/types/module"
)

type appModule struct {
	app *App
}

func (m appModule) RegisterServices(configurator module.Configurator) {
	err := m.app.registerRuntimeServices(configurator)
	if err != nil {
		panic(err)
	}
}

func (m appModule) IsOnePerModuleType() {}
func (m appModule) IsAppModule()        {}

var (
	_ appmodule.AppModule = appModule{}
	_ module.HasServices  = appModule{}
)

// BaseAppOption is a depinject.AutoGroupType which can be used to pass
// BaseApp options into the depinject. It should be used carefully.
type BaseAppOption func(*baseapp.BaseApp)

// IsManyPerContainerType indicates that this is a depinject.ManyPerContainerType.
func (b BaseAppOption) IsManyPerContainerType() {}

func init() {
	appmodule.Register(&runtimev1alpha1.Module{},
		appmodule.Provide(
			ProvideApp,
			ProvideKVStoreKey,
			ProvideTransientStoreKey,
			ProvideMemoryStoreKey,
			ProvideDeliverTx,
			ProvideKVStoreService,
			ProvideMemoryStoreService,
			ProvideTransientStoreService,
			ProvideInterModuleClient,
		),
		appmodule.Invoke(SetupAppBuilder),
	)
}

func ProvideApp() (
	codectypes.InterfaceRegistry,
	codec.Codec,
	*codec.LegacyAmino,
	*AppBuilder,
	codec.ProtoCodecMarshaler,
	*baseapp.MsgServiceRouter,
	appmodule.AppModule,
) {
	interfaceRegistry := codectypes.NewInterfaceRegistry()
	amino := codec.NewLegacyAmino()

	std.RegisterInterfaces(interfaceRegistry)
	std.RegisterLegacyAminoCodec(amino)

	cdc := codec.NewProtoCodec(interfaceRegistry)
	msgServiceRouter := baseapp.NewMsgServiceRouter()
	app := &AppBuilder{
		&App{
			storeKeys:         nil,
			interfaceRegistry: interfaceRegistry,
			cdc:               cdc,
			amino:             amino,
			basicManager:      basicManager,
			msgServiceRouter:  msgServiceRouter,
			BaseApp:           &baseapp.BaseApp{},
		},
	}
	appBuilder := &AppBuilder{app}

	return interfaceRegistry, cdc, amino, app, cdc
}

type AppInputs struct {
	depinject.In

	AppConfig         *appv1alpha1.Config
	Config            *runtimev1alpha1.Module
	AppBuilder        *AppBuilder
	Modules           map[string]appmodule.AppModule
	BaseAppOptions    []BaseAppOption
	InterfaceRegistry codectypes.InterfaceRegistry
	LegacyAmino       *codec.LegacyAmino
}

func SetupAppBuilder(inputs AppInputs) {
	app := inputs.AppBuilder.app
	app.baseAppOptions = inputs.BaseAppOptions
	app.config = inputs.Config
	app.ModuleManager = module.NewManagerFromMap(inputs.Modules)
	app.appConfig = inputs.AppConfig

	for name, mod := range inputs.Modules {
		if basicMod, ok := mod.(module.AppModuleBasic); ok {
			app.basicManager[name] = basicMod
			basicMod.RegisterInterfaces(inputs.InterfaceRegistry)
			basicMod.RegisterLegacyAminoCodec(inputs.LegacyAmino)
		}
	}
}

func registerStoreKey(wrapper *AppBuilder, key storetypes.StoreKey) {
	wrapper.app.storeKeys = append(wrapper.app.storeKeys, key)
}

func storeKeyOverride(config *runtimev1alpha1.Module, moduleName string) *runtimev1alpha1.StoreKeyConfig {
	for _, cfg := range config.OverrideStoreKeys {
		if cfg.ModuleName == moduleName {
			return cfg
		}
	}
	return nil
}

func ProvideKVStoreKey(config *runtimev1alpha1.Module, key depinject.ModuleKey, app *AppBuilder) *storetypes.KVStoreKey {
	override := storeKeyOverride(config, key.Name())

	var storeKeyName string
	if override != nil {
		storeKeyName = override.KvStoreKey
	} else {
		storeKeyName = key.Name()
	}

	storeKey := storetypes.NewKVStoreKey(storeKeyName)
	registerStoreKey(app, storeKey)
	return storeKey
}

func ProvideTransientStoreKey(key depinject.ModuleKey, app *AppBuilder) *storetypes.TransientStoreKey {
	storeKey := storetypes.NewTransientStoreKey(fmt.Sprintf("transient:%s", key.Name()))
	registerStoreKey(app, storeKey)
	return storeKey
}

func ProvideMemoryStoreKey(key depinject.ModuleKey, app *AppBuilder) *storetypes.MemoryStoreKey {
	storeKey := storetypes.NewMemoryStoreKey(fmt.Sprintf("memory:%s", key.Name()))
	registerStoreKey(app, storeKey)
	return storeKey
}

func ProvideDeliverTx(appBuilder *AppBuilder) func(abci.RequestDeliverTx) abci.ResponseDeliverTx {
	return func(tx abci.RequestDeliverTx) abci.ResponseDeliverTx {
		return appBuilder.app.BaseApp.DeliverTx(tx)
	}
}

func ProvideInterModuleClient(key depinject.ModuleKey, app *AppBuilder) intermodule.Client {
	return app.app.BaseApp.InterModuleClient(key.Name())
}