package collectors

import (
	"fmt"
	"time"

	"github.com/dmachard/go-dnscollector/dnsutils"
	"github.com/dmachard/go-dnscollector/transformers"
	"github.com/dmachard/go-logger"
	"github.com/miekg/dns"
)

func GetFakeDns() ([]byte, error) {
	dnsmsg := new(dns.Msg)
	dnsmsg.SetQuestion("dns.collector.", dns.TypeA)
	return dnsmsg.Pack()
}

type DnsProcessor struct {
	done         chan bool
	cleanup      chan bool
	cleanupDrops chan bool
	recvFrom     chan dnsutils.DnsMessage
	logger       *logger.Logger
	config       *dnsutils.Config
	name         string
	dropped      chan string
	droppedCount map[string]int
}

func NewDnsProcessor(config *dnsutils.Config, logger *logger.Logger, name string, size int) DnsProcessor {
	logger.Info("[%s] processor dns - initialization...", name)
	d := DnsProcessor{
		done:         make(chan bool),
		cleanup:      make(chan bool),
		cleanupDrops: make(chan bool),
		recvFrom:     make(chan dnsutils.DnsMessage, size),
		logger:       logger,
		config:       config,
		name:         name,
		dropped:      make(chan string),
		droppedCount: map[string]int{},
	}

	d.ReadConfig()

	return d
}

func (d *DnsProcessor) ReadConfig() {}

func (c *DnsProcessor) LogInfo(msg string, v ...interface{}) {
	c.logger.Info("["+c.name+"] dns processor - "+msg, v...)
}

func (c *DnsProcessor) LogError(msg string, v ...interface{}) {
	c.logger.Error("["+c.name+"] dns processor - "+msg, v...)
}

func (d *DnsProcessor) GetChannel() chan dnsutils.DnsMessage {
	return d.recvFrom
}

func (d *DnsProcessor) GetChannelList() []chan dnsutils.DnsMessage {
	channel := []chan dnsutils.DnsMessage{}
	channel = append(channel, d.recvFrom)
	return channel
}

func (d *DnsProcessor) Stop() {
	d.LogInfo("stopping...")

	// exit goroutine
	d.cleanup <- true

	// read done channel and block until run is terminated
	<-d.done
	d.LogInfo("run terminated")
	close(d.done)
}

func (d *DnsProcessor) DropsFollowing() {
	watchInterval := 10 * time.Second
	bufferFull := time.NewTimer(watchInterval)
	for {
		select {
		case <-d.cleanupDrops:
			return
		case loggerName := <-d.dropped:
			if _, ok := d.droppedCount[loggerName]; !ok {
				d.droppedCount[loggerName] = 1
			} else {
				d.droppedCount[loggerName]++
			}
		case <-bufferFull.C:

			for v, k := range d.droppedCount {
				if k > 0 {
					d.LogError("logger[%s] buffer is full, %d packet(s) dropped", v, k)
					d.droppedCount[v] = 0
				}
			}
			bufferFull.Reset(watchInterval)

		}
	}
}

func (d *DnsProcessor) Run(loggersChannel []chan dnsutils.DnsMessage, loggersName []string) {

	// prepare enabled transformers
	subprocessors := transformers.NewTransforms(&d.config.IngoingTransformers, d.logger, d.name, loggersChannel)

	// start goroutine to count dropped messsages
	go d.DropsFollowing()

	// read incoming dns message
	d.LogInfo("running... waiting dns message")
	for {
		select {
		case <-d.cleanup:
			d.LogInfo("cleanup called")
			close(d.recvFrom)

			// reset
			d.cleanupDrops <- true
			subprocessors.Reset()

			d.done <- true
			return

		case dm, opened := <-d.recvFrom:
			if !opened {
				d.LogInfo("channel closed, exit")
				d.cleanup <- true
				continue
			}

			// init dns message with additionnals parts
			subprocessors.InitDnsMessageFormat(&dm)

			// compute timestamp
			ts := time.Unix(int64(dm.DnsTap.TimeSec), int64(dm.DnsTap.TimeNsec))
			dm.DnsTap.Timestamp = ts.UnixNano()
			dm.DnsTap.TimestampRFC3339 = ts.UTC().Format(time.RFC3339Nano)

			// decode the dns payload
			dnsHeader, err := dnsutils.DecodeDns(dm.DNS.Payload)
			if err != nil {
				dm.DNS.MalformedPacket = true
				d.LogError("dns parser malformed packet: %s - %v+", err, dm)
			}

			// dns reply ?
			if dnsHeader.Qr == 1 {
				dm.DnsTap.Operation = "CLIENT_RESPONSE"
				dm.DNS.Type = dnsutils.DnsReply
				qip := dm.NetworkInfo.QueryIp
				qport := dm.NetworkInfo.QueryPort
				dm.NetworkInfo.QueryIp = dm.NetworkInfo.ResponseIp
				dm.NetworkInfo.QueryPort = dm.NetworkInfo.ResponsePort
				dm.NetworkInfo.ResponseIp = qip
				dm.NetworkInfo.ResponsePort = qport
			} else {
				dm.DNS.Type = dnsutils.DnsQuery
				dm.DnsTap.Operation = dnsutils.DNSTAP_CLIENT_QUERY
			}

			if err = dnsutils.DecodePayload(&dm, &dnsHeader, d.config); err != nil {
				d.LogError("%v - %v", err, dm)
			}

			if dm.DNS.MalformedPacket {
				if d.config.Global.Trace.LogMalformed {
					d.LogInfo("payload: %v", dm.DNS.Payload)
				}
			}

			// apply all enabled transformers
			if subprocessors.ProcessMessage(&dm) == transformers.RETURN_DROP {
				continue
			}

			// convert latency to human
			dm.DnsTap.LatencySec = fmt.Sprintf("%.6f", dm.DnsTap.Latency)

			// dispatch dns message to all generators
			for i := range loggersChannel {
				select {
				case loggersChannel[i] <- dm: // Successful send to logger channel
				default:
					d.dropped <- loggersName[i]
				}
			}
		}
	}
}
