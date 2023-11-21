package processors

import (
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/dmachard/go-dnscollector/dnsutils"
	"github.com/dmachard/go-dnscollector/transformers"
	"github.com/dmachard/go-dnstap-protobuf"
	"github.com/dmachard/go-logger"
	"google.golang.org/protobuf/proto"
)

func GetFakeDNSTap(dnsquery []byte) *dnstap.Dnstap {
	dtQuery := &dnstap.Dnstap{}

	dt := dnstap.Dnstap_MESSAGE
	dtQuery.Identity = []byte("dnstap-generator")
	dtQuery.Version = []byte("-")
	dtQuery.Type = &dt

	mt := dnstap.Message_CLIENT_QUERY
	sf := dnstap.SocketFamily_INET
	sp := dnstap.SocketProtocol_UDP

	now := time.Now()
	tsec := uint64(now.Unix())
	tnsec := uint32(uint64(now.UnixNano()) - uint64(now.Unix())*1e9)

	rport := uint32(53)
	qport := uint32(5300)

	msg := &dnstap.Message{Type: &mt}
	msg.SocketFamily = &sf
	msg.SocketProtocol = &sp
	msg.QueryAddress = net.ParseIP("127.0.0.1")
	msg.QueryPort = &qport
	msg.ResponseAddress = net.ParseIP("127.0.0.2")
	msg.ResponsePort = &rport

	msg.QueryMessage = dnsquery
	msg.QueryTimeSec = &tsec
	msg.QueryTimeNsec = &tnsec

	dtQuery.Message = msg
	return dtQuery
}

type DNSTapProcessor struct {
	ConnID       int
	doneRun      chan bool
	stopRun      chan bool
	doneMonitor  chan bool
	stopMonitor  chan bool
	recvFrom     chan []byte
	logger       *logger.Logger
	config       *dnsutils.Config
	ConfigChan   chan *dnsutils.Config
	name         string
	chanSize     int
	dropped      chan string
	droppedCount map[string]int
}

func NewDNSTapProcessor(connID int, config *dnsutils.Config, logger *logger.Logger, name string, size int) DNSTapProcessor {
	logger.Info("[%s] processor=dnstap#%d - initialization...", name, connID)

	d := DNSTapProcessor{
		ConnID:       connID,
		doneMonitor:  make(chan bool),
		doneRun:      make(chan bool),
		stopMonitor:  make(chan bool),
		stopRun:      make(chan bool),
		recvFrom:     make(chan []byte, size),
		chanSize:     size,
		logger:       logger,
		config:       config,
		ConfigChan:   make(chan *dnsutils.Config),
		name:         name,
		dropped:      make(chan string),
		droppedCount: map[string]int{},
	}

	return d
}

func (d *DNSTapProcessor) LogInfo(msg string, v ...interface{}) {
	var log string
	if d.ConnID == 0 {
		log = fmt.Sprintf("[%s] processor=dnstap - ", d.name)
	} else {
		log = fmt.Sprintf("[%s] processor=dnstap#%d - ", d.name, d.ConnID)
	}
	d.logger.Info(log+msg, v...)
}

func (d *DNSTapProcessor) LogError(msg string, v ...interface{}) {
	var log string
	if d.ConnID == 0 {
		log = fmt.Sprintf("[%s] processor=dnstap - ", d.name)
	} else {
		log = fmt.Sprintf("[%s] processor=dnstap#%d - ", d.name, d.ConnID)
	}
	d.logger.Error(log+msg, v...)
}

func (d *DNSTapProcessor) GetChannel() chan []byte {
	return d.recvFrom
}

func (d *DNSTapProcessor) Stop() {
	d.LogInfo("stopping to process...")
	d.stopRun <- true
	<-d.doneRun

	d.LogInfo("stopping to monitor loggers...")
	d.stopMonitor <- true
	<-d.doneMonitor
}

func (d *DNSTapProcessor) MonitorLoggers() {
	watchInterval := 10 * time.Second
	bufferFull := time.NewTimer(watchInterval)
MONITOR_LOOP:
	for {
		select {
		case <-d.stopMonitor:
			close(d.dropped)
			bufferFull.Stop()
			d.doneMonitor <- true
			break MONITOR_LOOP

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
	d.LogInfo("monitor terminated")
}

func (d *DNSTapProcessor) Run(loggersChannel []chan dnsutils.DNSMessage, loggersName []string) {
	dt := &dnstap.Dnstap{}

	// prepare enabled transformers
	transforms := transformers.NewTransforms(&d.config.IngoingTransformers, d.logger, d.name, loggersChannel, d.ConnID)

	// start goroutine to count dropped messsages
	go d.MonitorLoggers()

	// read incoming dns message
	d.LogInfo("waiting dns message to process...")
RUN_LOOP:
	for {
		select {
		case cfg := <-d.ConfigChan:
			d.config = cfg
			transforms.ReloadConfig(&cfg.IngoingTransformers)

		case <-d.stopRun:
			transforms.Reset()
			d.doneRun <- true
			break RUN_LOOP

		case data, opened := <-d.recvFrom:
			if !opened {
				d.LogInfo("channel closed, exit")
				return
			}

			err := proto.Unmarshal(data, dt)
			if err != nil {
				continue
			}

			// init dns message
			dm := dnsutils.DNSMessage{}
			dm.Init()

			// init dns message with additionnals parts
			transforms.InitDNSMessageFormat(&dm)

			identity := dt.GetIdentity()
			if len(identity) > 0 {
				dm.DNSTap.Identity = string(identity)
			}
			version := dt.GetVersion()
			if len(version) > 0 {
				dm.DNSTap.Version = string(version)
			}
			dm.DNSTap.Operation = dt.GetMessage().GetType().String()

			extra := string(dt.GetExtra())
			if len(extra) > 0 {
				dm.DNSTap.Extra = extra
			}

			if ipVersion, valid := dnsutils.IPVersion[dt.GetMessage().GetSocketFamily().String()]; valid {
				dm.NetworkInfo.Family = ipVersion
			} else {
				dm.NetworkInfo.Family = dnsutils.StrUnknown
			}

			dm.NetworkInfo.Protocol = dt.GetMessage().GetSocketProtocol().String()

			// decode query address and port
			queryip := dt.GetMessage().GetQueryAddress()
			if len(queryip) > 0 {
				dm.NetworkInfo.QueryIP = net.IP(queryip).String()
			}
			queryport := dt.GetMessage().GetQueryPort()
			if queryport > 0 {
				dm.NetworkInfo.QueryPort = strconv.FormatUint(uint64(queryport), 10)
			}

			// decode response address and port
			responseip := dt.GetMessage().GetResponseAddress()
			if len(responseip) > 0 {
				dm.NetworkInfo.ResponseIP = net.IP(responseip).String()
			}
			responseport := dt.GetMessage().GetResponsePort()
			if responseport > 0 {
				dm.NetworkInfo.ResponsePort = strconv.FormatUint(uint64(responseport), 10)
			}

			// get dns payload and timestamp according to the type (query or response)
			op := dnstap.Message_Type_value[dm.DNSTap.Operation]
			if op%2 == 1 {
				dnsPayload := dt.GetMessage().GetQueryMessage()
				dm.DNS.Payload = dnsPayload
				dm.DNS.Length = len(dnsPayload)
				dm.DNS.Type = dnsutils.DNSQuery
				dm.DNSTap.TimeSec = int(dt.GetMessage().GetQueryTimeSec())
				dm.DNSTap.TimeNsec = int(dt.GetMessage().GetQueryTimeNsec())
			} else {
				dnsPayload := dt.GetMessage().GetResponseMessage()
				dm.DNS.Payload = dnsPayload
				dm.DNS.Length = len(dnsPayload)
				dm.DNS.Type = dnsutils.DNSReply
				dm.DNSTap.TimeSec = int(dt.GetMessage().GetResponseTimeSec())
				dm.DNSTap.TimeNsec = int(dt.GetMessage().GetResponseTimeNsec())
			}

			// compute timestamp
			ts := time.Unix(int64(dm.DNSTap.TimeSec), int64(dm.DNSTap.TimeNsec))
			dm.DNSTap.Timestamp = ts.UnixNano()
			dm.DNSTap.TimestampRFC3339 = ts.UTC().Format(time.RFC3339Nano)

			if !d.config.Collectors.Dnstap.DisableDNSParser {
				// decode the dns payload to get id, rcode and the number of question
				// number of answer, ignore invalid packet
				dnsHeader, err := dnsutils.DecodeDNS(dm.DNS.Payload)
				if err != nil {
					// parser error
					dm.DNS.MalformedPacket = true
					d.LogInfo("dns parser malformed packet: %s", err)
				}

				if err = dnsutils.DecodePayload(&dm, &dnsHeader, d.config); err != nil {
					// decoding error
					if d.config.Global.Trace.LogMalformed {
						d.LogError("%v - %v", err, dm)
						d.LogError("dump invalid dns payload: %v", dm.DNS.Payload)
					}
				}
			}

			// apply all enabled transformers
			if transforms.ProcessMessage(&dm) == transformers.ReturnDrop {
				continue
			}

			// convert latency to human
			dm.DNSTap.LatencySec = fmt.Sprintf("%.6f", dm.DNSTap.Latency)

			// dispatch dns message to connected loggers
			for i := range loggersChannel {
				select {
				case loggersChannel[i] <- dm: // Successful send to logger channel
				default:
					d.dropped <- loggersName[i]
				}
			}

		}
	}

	d.LogInfo("processing terminated")
}