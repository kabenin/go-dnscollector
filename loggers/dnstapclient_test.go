package loggers

import (
	"bufio"
	"net"
	"testing"
	"time"

	"github.com/dmachard/go-dnscollector/dnsutils"
	"github.com/dmachard/go-dnscollector/pkgconfig"
	"github.com/dmachard/go-dnstap-protobuf"
	"github.com/dmachard/go-framestream"
	"github.com/dmachard/go-logger"
	"google.golang.org/protobuf/proto"
)

func Test_DnstapClient(t *testing.T) {

	testcases := []struct {
		transport string
		address   string
	}{
		{
			transport: "tcp",
			address:   ":6000",
		},
		{
			transport: "unix",
			address:   "/tmp/test.sock",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.transport, func(t *testing.T) {
			// init logger
			cfg := pkgconfig.GetFakeConfig()
			cfg.Loggers.DNSTap.FlushInterval = 1
			cfg.Loggers.DNSTap.BufferSize = 0
			if tc.transport == "unix" {
				cfg.Loggers.DNSTap.SockPath = tc.address
			}

			g := NewDnstapSender(cfg, logger.New(false), "test")

			// fake dnstap receiver
			fakeRcvr, err := net.Listen(tc.transport, tc.address)
			if err != nil {
				t.Fatal(err)
			}
			defer fakeRcvr.Close()

			// start the logger
			go g.Run()

			// accept conn from logger
			conn, err := fakeRcvr.Accept()
			if err != nil {
				return
			}
			defer conn.Close()

			// init framestream on server side
			fsSvr := framestream.NewFstrm(bufio.NewReader(conn), bufio.NewWriter(conn), conn, 5*time.Second, []byte("protobuf:dnstap.Dnstap"), true)
			if err := fsSvr.InitReceiver(); err != nil {
				t.Errorf("error to init framestream receiver: %s", err)
			}

			// wait framestream to be ready
			time.Sleep(time.Second)

			// send fake dns message to logger
			dm := dnsutils.GetFakeDNSMessage()
			g.GetInputChannel() <- dm

			// receive frame on server side ?, timeout 5s
			fs, err := fsSvr.RecvFrame(true)
			if err != nil {
				t.Errorf("error to receive frame: %s", err)
			}

			// decode the dnstap message in server side
			dt := &dnstap.Dnstap{}
			if err := proto.Unmarshal(fs.Data(), dt); err != nil {
				t.Errorf("error to decode dnstap")
			}
		})
	}
}
