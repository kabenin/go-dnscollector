package subprocessors

import (
	"sync"

	"github.com/dmachard/go-dnscollector/dnsutils"
	"github.com/dmachard/go-topmap"
)

type StatsStreams struct {
	streams map[string]*StatsPerStream
	config  *dnsutils.Config
	sync.RWMutex
}

func NewStreamsStats(config *dnsutils.Config) *StatsStreams {
	c := &StatsStreams{
		config:  config,
		streams: make(map[string]*StatsPerStream),
	}
	c.streams["global"] = NewStatsPerStream(config)
	return c
}

func (c *StatsStreams) Record(dm dnsutils.DnsMessage) {
	c.Lock()
	defer c.Unlock()

	// global record
	c.streams["global"].Record(dm)

	// record for each ident
	if _, ok := c.streams[dm.Identity]; !ok {
		c.streams[dm.Identity] = NewStatsPerStream(c.config)
	}
	c.streams[dm.Identity].Record(dm)
}

func (c *StatsStreams) Streams() []string {
	c.RLock()
	defer c.RUnlock()

	ret := []string{}
	for k, _ := range c.streams {
		ret = append(ret, k)
	}
	return ret
}

func (c *StatsStreams) Compute() {
	c.Lock()
	for _, v := range c.streams {
		v.Compute()
	}
	c.Unlock()
}

func (c *StatsStreams) GetCounters(identity string) (ret Counters) {
	c.RLock()
	defer c.RUnlock()

	return c.streams[identity].GetCounters()
}

func (c *StatsStreams) GetTotalDomains(identity string) (ret int) {
	c.RLock()
	defer c.RUnlock()

	v, found := c.streams[identity]
	if !found {
		return 0
	}

	return v.GetTotalDomains()
}

func (c *StatsStreams) GetTotalNxdomains(identity string) (ret int) {
	c.RLock()
	defer c.RUnlock()

	v, found := c.streams[identity]
	if !found {
		return 0
	}

	return v.GetTotalNxdomains()
}

func (c *StatsStreams) GetTotalSlowdomains(identity string) (ret int) {
	c.RLock()
	defer c.RUnlock()

	v, found := c.streams[identity]
	if !found {
		return 0
	}

	return v.GetTotalSlowdomains()
}

func (c *StatsStreams) GetTotalSuspiciousdomains(identity string) (ret int) {
	c.RLock()
	defer c.RUnlock()

	v, found := c.streams[identity]
	if !found {
		return 0
	}

	return v.GetTotalSuspiciousdomains()
}

func (c *StatsStreams) GetTotalClients(identity string) (ret int) {
	c.RLock()
	defer c.RUnlock()

	v, found := c.streams[identity]
	if !found {
		return 0
	}

	return v.GetTotalClients()
}

func (c *StatsStreams) GetTopQnames(identity string) (ret []topmap.TopMapItem) {
	c.RLock()
	defer c.RUnlock()

	v, found := c.streams[identity]
	if !found {
		return []topmap.TopMapItem{}
	}

	return v.GetTopQnames()
}

func (c *StatsStreams) GetTopNxdomains(identity string) (ret []topmap.TopMapItem) {
	c.RLock()
	defer c.RUnlock()

	v, found := c.streams[identity]
	if !found {
		return []topmap.TopMapItem{}
	}

	return v.GetTopNxdomains()
}

func (c *StatsStreams) GetTopSlowdomains(identity string) (ret []topmap.TopMapItem) {
	c.RLock()
	defer c.RUnlock()

	v, found := c.streams[identity]
	if !found {
		return []topmap.TopMapItem{}
	}

	return v.GetTopSlowdomains()
}

func (c *StatsStreams) GetTopSuspiciousdomains(identity string) (ret []topmap.TopMapItem) {
	c.RLock()
	defer c.RUnlock()

	v, found := c.streams[identity]
	if !found {
		return []topmap.TopMapItem{}
	}

	return v.GetTopSuspiciousdomains()
}

func (c *StatsStreams) GetTopClients(identity string) (ret []topmap.TopMapItem) {
	c.RLock()
	defer c.RUnlock()

	v, found := c.streams[identity]
	if !found {
		return []topmap.TopMapItem{}
	}

	return v.GetTopClients()
}

func (c *StatsStreams) GetTopRcodes(identity string) (ret []topmap.TopMapItem) {
	c.RLock()
	defer c.RUnlock()

	v, found := c.streams[identity]
	if !found {
		return []topmap.TopMapItem{}
	}
	return v.GetTopRcodes()
}

func (c *StatsStreams) GetTopRrtypes(identity string) (ret []topmap.TopMapItem) {
	c.RLock()
	defer c.RUnlock()

	v, found := c.streams[identity]
	if !found {
		return []topmap.TopMapItem{}
	}

	return v.GetTopRrtypes()
}

func (c *StatsStreams) GetTopOperations(identity string) (ret []topmap.TopMapItem) {
	c.RLock()
	defer c.RUnlock()

	v, found := c.streams[identity]
	if !found {
		return []topmap.TopMapItem{}
	}

	return v.GetTopOperations()
}

func (c *StatsStreams) GetTopTransports(identity string) (ret []topmap.TopMapItem) {
	c.RLock()
	defer c.RUnlock()

	v, found := c.streams[identity]
	if !found {
		return []topmap.TopMapItem{}
	}

	return v.GetTopTransports()
}

func (c *StatsStreams) GetTopIpProto(identity string) (ret []topmap.TopMapItem) {
	c.RLock()
	defer c.RUnlock()

	v, found := c.streams[identity]
	if !found {
		return []topmap.TopMapItem{}
	}

	return v.GetTopIpProto()
}