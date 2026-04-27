package config

import "sync"

type Runtime struct {
	mu                    sync.RWMutex
	batchLoginConcurrency int
	autoRefresh           bool
	autoRefreshInterval   int
	outThink              bool
	searchInfoMode        string
	simpleModelMap        bool
}

func NewRuntime(cfg Config) *Runtime {
	return &Runtime{
		batchLoginConcurrency: cfg.BatchLoginConcurrency,
		autoRefresh:           cfg.AutoRefresh,
		autoRefreshInterval:   cfg.AutoRefreshInterval,
		outThink:              cfg.OutThink,
		searchInfoMode:        cfg.SearchInfoMode,
		simpleModelMap:        cfg.SimpleModelMap,
	}
}

func (r *Runtime) Snapshot() RuntimeSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return RuntimeSnapshot{
		BatchLoginConcurrency: r.batchLoginConcurrency,
		AutoRefresh:           r.autoRefresh,
		AutoRefreshInterval:   r.autoRefreshInterval,
		OutThink:              r.outThink,
		SearchInfoMode:        r.searchInfoMode,
		SimpleModelMap:        r.simpleModelMap,
	}
}

type RuntimeSnapshot struct {
	BatchLoginConcurrency int
	AutoRefresh           bool
	AutoRefreshInterval   int
	OutThink              bool
	SearchInfoMode        string
	SimpleModelMap        bool
}

func (r *Runtime) SetAutoRefresh(enabled bool, interval int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.autoRefresh = enabled
	r.autoRefreshInterval = interval
}

func (r *Runtime) SetBatchLoginConcurrency(v int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.batchLoginConcurrency = v
}

func (r *Runtime) SetOutThink(v bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.outThink = v
}

func (r *Runtime) SetSearchInfoMode(v string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.searchInfoMode = v
}

func (r *Runtime) SetSimpleModelMap(v bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.simpleModelMap = v
}

func (r *Runtime) ApplySnapshot(snapshot RuntimeSnapshot) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.batchLoginConcurrency = snapshot.BatchLoginConcurrency
	r.autoRefresh = snapshot.AutoRefresh
	r.autoRefreshInterval = snapshot.AutoRefreshInterval
	r.outThink = snapshot.OutThink
	r.searchInfoMode = snapshot.SearchInfoMode
	r.simpleModelMap = snapshot.SimpleModelMap
}
