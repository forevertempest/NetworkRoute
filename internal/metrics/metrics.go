package metrics

import "sync/atomic"

type Counters struct {
	Active    atomic.Int64
	Direct    atomic.Uint64
	Relayed   atomic.Uint64
	Bytes     atomic.Uint64
	Datagrams atomic.Uint64
	Dropped   atomic.Uint64
	Fallbacks atomic.Uint64
}

func (m *Counters) Snapshot() map[string]any {
	return map[string]any{"flows_active": m.Active.Load(), "flows_direct_total": m.Direct.Load(), "flows_relayed_total": m.Relayed.Load(), "bytes_forwarded": m.Bytes.Load(), "datagrams_forwarded": m.Datagrams.Load(), "datagrams_dropped": m.Dropped.Load(), "new_flow_fallbacks": m.Fallbacks.Load()}
}
