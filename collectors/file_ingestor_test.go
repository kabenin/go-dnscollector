package collectors

import (
	"testing"

	"github.com/dmachard/go-dnscollector/dnsutils"
	"github.com/dmachard/go-dnscollector/pkgconfig"
	"github.com/dmachard/go-dnscollector/pkgutils"
	"github.com/dmachard/go-logger"
)

func Test_FileIngestor_Pcap(t *testing.T) {
	g := pkgutils.NewFakeLogger()
	config := pkgconfig.GetFakeConfig()

	// watch tests data folder
	config.Collectors.FileIngestor.WatchDir = "./../testsdata/pcap/"

	// init collector
	c := NewFileIngestor([]pkgutils.Worker{g}, config, logger.New(false), "test")
	go c.Run()

	// waiting message in channel
	for {
		// read dns message from channel
		msg := <-g.GetInputChannel()

		// check qname
		if msg.DNSTap.Operation == dnsutils.DNSTapClientQuery {
			break
		}
	}
}
