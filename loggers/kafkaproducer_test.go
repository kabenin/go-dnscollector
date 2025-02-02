package loggers

import (
	"log"
	"net"
	"testing"
	"time"

	"github.com/dmachard/go-dnscollector/dnsutils"
	"github.com/dmachard/go-dnscollector/pkgconfig"
	"github.com/dmachard/go-logger"

	sarama "github.com/Shopify/sarama"
)

func Test_KafkaProducer(t *testing.T) {

	// for debug only
	// sarama.Logger = log.New(os.Stdout, "[sarama] ", log.LstdFlags)

	testcases := []struct {
		transport string
		address   string
		topic     string
		compress  string
	}{
		{
			transport: "compress_none",
			address:   ":9092",
			topic:     "dnscollector",
			compress:  "none",
		},
		{
			transport: "compress_gzip",
			address:   ":9092",
			topic:     "dnscollector",
			compress:  "gzip",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.transport, func(t *testing.T) {
			// Create a new mock broker
			mockListener, err := net.Listen("tcp", "127.0.0.1:9092")
			if err != nil {
				log.Fatal(err)
			}
			defer mockListener.Close()

			// init logger
			cfg := pkgconfig.GetFakeConfig()
			cfg.Loggers.KafkaProducer.BufferSize = 0
			cfg.Loggers.KafkaProducer.RemotePort = 9092
			cfg.Loggers.KafkaProducer.Topic = tc.topic
			cfg.Loggers.KafkaProducer.Compression = tc.compress

			mockBroker := sarama.NewMockBrokerListener(t, 1, mockListener)
			defer mockBroker.Close()

			mockBroker.SetHandlerByMap(map[string]sarama.MockResponse{
				"ApiVersionsRequest": sarama.NewMockApiVersionsResponse(t).SetApiKeys(
					[]sarama.ApiVersionsResponseKey{
						{
							ApiKey:     3, // Metadata
							MinVersion: 0,
							MaxVersion: 6,
						},
						{
							ApiKey:     0, // Produce
							MinVersion: 0,
							MaxVersion: 7,
						},
					},
				),
				"MetadataRequest": sarama.NewMockMetadataResponse(t).
					SetBroker(mockBroker.Addr(), mockBroker.BrokerID()).
					SetController(mockBroker.BrokerID()).
					SetLeader(tc.topic, 0, mockBroker.BrokerID()),
				"ProduceRequest": sarama.NewMockProduceResponse(t).
					SetError(tc.topic, 0, sarama.ErrNoError).
					SetVersion(6),
			})

			// start the logger
			g := NewKafkaProducer(cfg, logger.New(false), "test")
			go g.Run()

			// wait connection
			time.Sleep(1 * time.Second)

			// send fake dns message to logger
			dm := dnsutils.GetFakeDNSMessage()
			g.GetInputChannel() <- dm

			// just wait
			time.Sleep(1 * time.Second)

			// read history to find produce request
			produceRequest := false
			for i := 0; i < len(mockBroker.History()); i++ {
				rr := mockBroker.History()[i]
				if _, ok := rr.Request.(*sarama.ProduceRequest); !ok {
					continue
				}
				produceRequest = true

			}

			if !produceRequest {
				t.Errorf("ProduceRequest not received on broker")
			}
		})
	}

}
