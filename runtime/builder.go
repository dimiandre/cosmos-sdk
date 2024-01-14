package runtime

import (
	"encoding/json"
	"fmt"

	coreappmanager "cosmossdk.io/server/v2/core/appmanager"
	"cosmossdk.io/server/v2/core/mempool"
	"cosmossdk.io/server/v2/core/store"
	servertx "cosmossdk.io/server/v2/core/transaction"
	"cosmossdk.io/server/v2/stf"
	"cosmossdk.io/server/v2/stf/branch"
	"github.com/cosmos/cosmos-sdk/types/module"
)

// AppBuilder is a type that is injected into a container by the runtime module
// (as *AppBuilder) which can be used to create an app which is compatible with
// the existing app.go initialization conventions.
type AppBuilder struct {
	app *App
}

// DefaultGenesis returns a default genesis from the registered AppModuleBasic's.
func (a *AppBuilder) DefaultGenesis() map[string]json.RawMessage {
	return a.app.DefaultGenesis()
}

// RegisterModules registers the provided modules with the module manager and
// the basic module manager. This is the primary hook for integrating with
// modules which are not registered using the app config.
func (a *AppBuilder) RegisterModules(modules ...module.AppModule) error {
	for _, appModule := range modules {
		name := appModule.Name()
		if _, ok := a.app.moduleManager.Modules[name]; ok {
			return fmt.Errorf("AppModule named %q already exists", name)
		}

		if _, ok := a.app.basicManager[name]; ok {
			return fmt.Errorf("AppModuleBasic named %q already exists", name)
		}

		a.app.moduleManager.Modules[name] = appModule
		a.app.basicManager[name] = appModule
		appModule.RegisterInterfaces(a.app.interfaceRegistry)
		appModule.RegisterLegacyAminoCodec(a.app.amino)

		// if module, ok := appModule.(module.HasServices); ok {
		// 	module.RegisterServices(a.app.configurator)
		// } else if module, ok := appModule.(appmodule.HasServices); ok {
		// 	if err := module.RegisterServices(a.app.configurator); err != nil {
		// 		return err
		// 	}
		// }

		// moduleMsgRouter := _newModuleMsgRouter(name, s.msgRouterBuilder)
		// m.RegisterMsgHandlers(moduleMsgRouter)
		// m.RegisterPreMsgHandler(moduleMsgRouter)
		// m.RegisterPostMsgHandler(moduleMsgRouter)
		// // build query handler
		// moduleQueryRouter := _newModuleMsgRouter(name, s.queryRouterBuilder)
		// m.RegisterQueryHandler(moduleQueryRouter)
	}

	return nil
}

// Build builds an *App instance.
func (a *AppBuilder) Build(db store.Store, opts ...AppBuilderOption) (*App, error) {
	for _, opt := range opts {
		opt(a)
	}

	if err := a.app.moduleManager.RegisterServicesV2(a.app.msgRouterBuilder); err != nil {
		return nil, err
	}
	stfMsgHandler, err := a.app.msgRouterBuilder.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build STF message handler: %w", err)
	}

	upgradeBlocker := a.app.moduleManager.UpgradeBlockerV2()
	beginBlocker := a.app.moduleManager.BeginBlockV2()
	endBlocker, valUpdate := a.app.moduleManager.EndBlockV2()

	a.app.stf = stf.NewSTF[servertx.Tx](
		stfMsgHandler,
		stfMsgHandler,
		upgradeBlocker,
		beginBlocker,
		endBlocker,
		valUpdate,
		func(txBytes []byte) (servertx.Tx, error) { // TODO
			// return txCodec.Decode(txBytes)
			return nil, nil
		},
		func(state store.ReadonlyState) store.WritableState { // TODO
			return branch.NewStore(state)
		},
	)
	a.app.db = db

	return a.app, nil
}

// AppBuilderOption is a function that can be passed to AppBuilder.Build to
// customize the resulting app.
type AppBuilderOption func(*AppBuilder)

func AppBuilderWithMempool(mempool mempool.Mempool[servertx.Tx]) AppBuilderOption {
	return func(a *AppBuilder) {
		a.app.mempool = mempool
	}
}

func AppBuilderWithPrepareBlockHandler(handler coreappmanager.PrepareHandler[servertx.Tx]) AppBuilderOption {
	return func(a *AppBuilder) {
		a.app.prepareBlockHandler = handler
	}
}

func AppBuilderWithVerifyBlockHandler(handler coreappmanager.ProcessHandler[servertx.Tx]) AppBuilderOption {
	return func(a *AppBuilder) {
		a.app.verifyBlockHandler = handler
	}
}
