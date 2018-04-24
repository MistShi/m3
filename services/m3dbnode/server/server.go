	"github.com/m3db/m3cluster/client/etcd"
	"github.com/coreos/etcd/embed"
	bootstrapConfigInitTimeout        = 10 * time.Second
	serverGracefulCloseTimeout        = 10 * time.Second
	defaultNamespaceResolutionTimeout = time.Minute
	defaultTopologyResolutionTimeout  = time.Minute
	// EmbeddedKVBootstrapCh is a channel to listen on to be notified that the embedded KV has bootstrapped.
	EmbeddedKVBootstrapCh chan<- struct{}

	hostID, err := cfg.HostID.Resolve()
	if err != nil {
		logger.Fatalf("could not resolve local host ID: %v", err)
	}

	// Presence of KV server config indicates embedded etcd cluster
	if cfg.EnvironmentConfig.SeedNodes != nil {
		// Default etcd client clusters if not set already
		clusters := cfg.EnvironmentConfig.Service.ETCDClusters
		if len(clusters) == 0 {
			endpoints, err := config.InitialClusterEndpoints(cfg.EnvironmentConfig.SeedNodes.InitialCluster)

			if err != nil {
				logger.Fatalf("unable to create etcd clusters: %v", err)
			}

			zone := cfg.EnvironmentConfig.Service.Zone

			logger.Infof("using seed nodes etcd cluster: zone=%s, endpoints=%v", zone, endpoints)

			cfg.EnvironmentConfig.Service.ETCDClusters = []etcd.ClusterConfig{etcd.ClusterConfig{
				Zone:      zone,
				Endpoints: endpoints,
			}}
		}

		if config.IsSeedNode(cfg.EnvironmentConfig.SeedNodes.InitialCluster, hostID) {
			logger.Info("is a seed node; starting etcd server")

			etcdCfg, err := config.NewEtcdEmbedConfig(cfg)
			if err != nil {
				logger.Fatalf("unable to create etcd config: %v", err)
			}

			e, err := embed.StartEtcd(etcdCfg)
			if err != nil {
				logger.Fatalf("could not start embedded etcd: %v", err)
			}

			if runOpts.EmbeddedKVBootstrapCh != nil {
				// Notify on embedded KV bootstrap chan if specified
				runOpts.EmbeddedKVBootstrapCh <- struct{}{}
			}

			defer e.Close()
		}
	}

	opts := storage.NewOptions().
		SetIndexingEnabled(cfg.Index.Enabled)
	if lruCfg := cfg.Cache.SeriesConfiguration().LRU; lruCfg != nil {
		runtimeOpts = runtimeOpts.SetMaxWiredBlocks(lruCfg.MaxBlocks)
	}
	if cfg.EnvironmentConfig.Static == nil {
		namespaceResolutionTimeout := cfg.EnvironmentConfig.NamespaceResolutionTimeout
		if namespaceResolutionTimeout <= 0 {
			namespaceResolutionTimeout = defaultNamespaceResolutionTimeout
		}

		topologyResolutionTimeout := cfg.EnvironmentConfig.TopologyResolutionTimeout
		if topologyResolutionTimeout <= 0 {
			topologyResolutionTimeout = defaultTopologyResolutionTimeout
		}

			InstrumentOpts:             iopts,
			HashingSeed:                cfg.Hashing.Seed,
			NamespaceResolutionTimeout: namespaceResolutionTimeout,
			TopologyResolutionTimeout:  topologyResolutionTimeout,
	} else {
			InstrumentOpts: iopts,
			HostID:         hostID,
		func(opts client.AdminOptions) client.AdminOptions {
			return opts.SetRuntimeOptionsManager(runtimeOptsMgr).(client.AdminOptions)
		},
	// Kick off runtime options manager KV watches
	clientAdminOpts := m3dbClient.Options().(client.AdminOptions)
	kvWatchClientConsistencyLevels(envCfg.KVStore, logger,
		clientAdminOpts, runtimeOptsMgr)

			value := defaultClusterNewSeriesLimit
			if newValue := watch.Get(); newValue != nil {
				if err := newValue.Unmarshal(protoValue); err != nil {
					logger.Warnf("unable to parse new cluster new series insert limit: %v", err)
					continue
				}
				value = int(protoValue.Value)
			}

			err = setNewSeriesLimitPerShardOnChange(topo, runtimeOptsMgr, value)
		}
	}()
}
func kvWatchClientConsistencyLevels(
	store kv.Store,
	logger xlog.Logger,
	clientOpts client.AdminOptions,
	runtimeOptsMgr m3dbruntime.OptionsManager,
) {
	setReadConsistencyLevel := func(
		v string,
		applyFn func(topology.ReadConsistencyLevel, m3dbruntime.Options) m3dbruntime.Options,
	) error {
		for _, level := range topology.ValidReadConsistencyLevels() {
			if level.String() == v {
				runtimeOpts := applyFn(level, runtimeOptsMgr.Get())
				return runtimeOptsMgr.Update(runtimeOpts)
			}
		}
		return fmt.Errorf("invalid read consistency level set: %s", v)
	}

	setConsistencyLevel := func(
		v string,
		applyFn func(topology.ConsistencyLevel, m3dbruntime.Options) m3dbruntime.Options,
	) error {
		for _, level := range topology.ValidConsistencyLevels() {
			if level.String() == v {
				runtimeOpts := applyFn(level, runtimeOptsMgr.Get())
				return runtimeOptsMgr.Update(runtimeOpts)
			}
		}
		return fmt.Errorf("invalid consistency level set: %s", v)
	}

	kvWatchStringValue(store, logger,
		kvconfig.ClientBootstrapConsistencyLevel,
		func(value string) error {
			return setReadConsistencyLevel(value,
				func(level topology.ReadConsistencyLevel, opts m3dbruntime.Options) m3dbruntime.Options {
					return opts.SetClientBootstrapConsistencyLevel(level)
				})
		},
		func() error {
			return runtimeOptsMgr.Update(runtimeOptsMgr.Get().
				SetClientBootstrapConsistencyLevel(clientOpts.BootstrapConsistencyLevel()))
		})

	kvWatchStringValue(store, logger,
		kvconfig.ClientReadConsistencyLevel,
		func(value string) error {
			return setReadConsistencyLevel(value,
				func(level topology.ReadConsistencyLevel, opts m3dbruntime.Options) m3dbruntime.Options {
					return opts.SetClientReadConsistencyLevel(level)
				})
		},
		func() error {
			return runtimeOptsMgr.Update(runtimeOptsMgr.Get().
				SetClientReadConsistencyLevel(clientOpts.ReadConsistencyLevel()))
		})

	kvWatchStringValue(store, logger,
		kvconfig.ClientWriteConsistencyLevel,
		func(value string) error {
			return setConsistencyLevel(value,
				func(level topology.ConsistencyLevel, opts m3dbruntime.Options) m3dbruntime.Options {
					return opts.SetClientWriteConsistencyLevel(level)
				})
		},
		func() error {
			return runtimeOptsMgr.Update(runtimeOptsMgr.Get().
				SetClientWriteConsistencyLevel(clientOpts.WriteConsistencyLevel()))
		})
}

func kvWatchStringValue(
	store kv.Store,
	logger xlog.Logger,
	key string,
	onValue func(value string) error,
	onDelete func() error,
) {
	protoValue := &commonpb.StringProto{}

	// First try to eagerly set the value so it doesn't flap if the
	// watch returns but not immediately for an existing value
	value, err := store.Get(key)
	if err != nil && err != kv.ErrNotFound {
		logger.Errorf("could not resolve KV key %s: %v", key, err)
	}
	if err == nil {
		if err := value.Unmarshal(protoValue); err != nil {
			logger.Errorf("could not unmarshal KV key %s: %v", key, err)
		} else if err := onValue(protoValue.Value); err != nil {
			logger.Errorf("could not process value of KV key %s: %v", key, err)
		} else {
			logger.Infof("set KV key %s: %v", key, protoValue.Value)
		}
	}

	watch, err := store.Watch(key)
	if err != nil {
		logger.Errorf("could not watch KV key %s: %v", key, err)
		return
	}

	go func() {
		for range watch.C() {
			newValue := watch.Get()
			if newValue == nil {
				if err := onDelete(); err != nil {
					logger.Warnf("could not set default for KV key %s: %v", key, err)
				}
				continue
			}

			err := newValue.Unmarshal(protoValue)
				logger.Warnf("could not unmarshal KV key %s: %v", key, err)
				continue
			}
			if err := onValue(protoValue.Value); err != nil {
				logger.Warnf("could not process change for KV key %s: %v", key, err)
			logger.Infof("set KV key %s: %v", key, protoValue.Value)

	if opts.SeriesCachePolicy() == series.CacheLRU {
		runtimeOpts := opts.RuntimeOptionsManager()
		wiredList := block.NewWiredList(runtimeOpts, iopts, opts.ClockOptions())
		blockOpts = blockOpts.SetWiredList(wiredList)
	}