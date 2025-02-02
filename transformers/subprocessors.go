package transformers

import (
	"fmt"

	"github.com/dmachard/go-dnscollector/dnsutils"
	"github.com/dmachard/go-dnscollector/pkgconfig"
	"github.com/dmachard/go-dnscollector/pkgutils"
	"github.com/dmachard/go-logger"
)

const (
	enabled = "enabled"
)

var (
	ReturnSuccess = 1
	ReturnDrop    = 2
	ReturnError   = 3
)

type Transforms struct {
	config   *pkgconfig.ConfigTransformers
	logger   *logger.Logger
	name     string
	instance int

	SuspiciousTransform      SuspiciousTransform
	GeoipTransform           GeoIPProcessor
	FilteringTransform       FilteringProcessor
	UserPrivacyTransform     UserPrivacyProcessor
	NormalizeTransform       NormalizeProcessor
	LatencyTransform         *LatencyProcessor
	ReducerTransform         *ReducerProcessor
	ExtractProcessor         ExtractProcessor
	MachineLearningTransform MlProcessor
	ATagsTransform           ATagsProcessor

	activeTransforms []func(dm *dnsutils.DNSMessage) int
}

func NewTransforms(config *pkgconfig.ConfigTransformers, logger *logger.Logger, name string, outChannels []chan dnsutils.DNSMessage, instance int) Transforms {

	d := Transforms{
		config:   config,
		logger:   logger,
		name:     name,
		instance: instance,
	}

	d.SuspiciousTransform = NewSuspiciousSubprocessor(config, logger, name, instance, outChannels, d.LogInfo, d.LogError)
	d.NormalizeTransform = NewNormalizeSubprocessor(config, logger, name, instance, outChannels, d.LogInfo, d.LogError)
	d.ExtractProcessor = NewExtractSubprocessor(config, logger, name, instance, outChannels, d.LogInfo, d.LogError)
	d.LatencyTransform = NewLatencySubprocessor(config, logger, name, instance, outChannels, d.LogInfo, d.LogError)
	d.ReducerTransform = NewReducerSubprocessor(config, logger, name, instance, outChannels, d.LogInfo, d.LogError)
	d.UserPrivacyTransform = NewUserPrivacySubprocessor(config, logger, name, instance, outChannels, d.LogInfo, d.LogError)
	d.FilteringTransform = NewFilteringProcessor(config, logger, name, instance, outChannels, d.LogInfo, d.LogError)
	d.GeoipTransform = NewDNSGeoIPProcessor(config, logger, name, instance, outChannels, d.LogInfo, d.LogError)
	d.MachineLearningTransform = NewMachineLearningSubprocessor(config, logger, name, instance, outChannels, d.LogInfo, d.LogError)
	d.ATagsTransform = NewATagsSubprocessor(config, logger, name, instance, outChannels, d.LogInfo, d.LogError)

	d.Prepare()
	return d
}

func (p *Transforms) ReloadConfig(config *pkgconfig.ConfigTransformers) {
	p.config = config
	p.NormalizeTransform.ReloadConfig(config)
	p.GeoipTransform.ReloadConfig(config)
	p.FilteringTransform.ReloadConfig(config)
	p.UserPrivacyTransform.ReloadConfig(config)
	p.LatencyTransform.ReloadConfig(config)
	p.SuspiciousTransform.ReloadConfig(config)
	p.ReducerTransform.ReloadConfig(config)
	p.ExtractProcessor.ReloadConfig(config)
	p.MachineLearningTransform.ReloadConfig(config)
	p.ATagsTransform.ReloadConfig(config)

	p.Prepare()
}

func (p *Transforms) Prepare() error {
	// clean the slice
	p.activeTransforms = p.activeTransforms[:0]

	var prefixlog string
	if p.instance > 0 {
		prefixlog = fmt.Sprintf("conn #%d - ", p.instance)
	} else {
		prefixlog = ""
	}

	if p.config.Normalize.Enable {
		p.LogInfo(prefixlog + "qname lowercase subprocessor is " + enabled)

		p.NormalizeTransform.LoadActiveProcessors()
	}

	if p.config.GeoIP.Enable {
		p.activeTransforms = append(p.activeTransforms, p.geoipTransform)
		p.LogInfo(prefixlog + "geoip subprocessor is " + enabled)

		if err := p.GeoipTransform.Open(); err != nil {
			p.LogError(prefixlog+"geoip subprocessor open error %v", err)
		}
	}

	if p.config.UserPrivacy.Enable {
		// Apply user privacy on qname and query ip
		if p.config.UserPrivacy.AnonymizeIP {
			p.activeTransforms = append(p.activeTransforms, p.anonymizeIP)
			p.LogInfo(prefixlog + "ip anonymization subprocessor is enabled")
		}

		if p.config.UserPrivacy.MinimazeQname {
			p.activeTransforms = append(p.activeTransforms, p.minimazeQname)
			p.LogInfo(prefixlog + "minimaze qname subprocessor is enabled")
		}

		if p.config.UserPrivacy.HashIP {
			p.activeTransforms = append(p.activeTransforms, p.hashIP)
			p.LogInfo(prefixlog + "hash ip subprocessor is enabled")
		}
	}

	if p.config.Filtering.Enable {
		p.LogInfo(prefixlog + "filtering subprocessor is " + enabled)

		p.FilteringTransform.LoadRcodes()
		p.FilteringTransform.LoadDomainsList()
		p.FilteringTransform.LoadQueryIPList()
		p.FilteringTransform.LoadrDataIPList()

		p.FilteringTransform.LoadActiveFilters()
	}

	if p.config.Latency.Enable {
		if p.config.Latency.MeasureLatency {
			p.activeTransforms = append(p.activeTransforms, p.measureLatency)
			p.LogInfo(prefixlog + "measure latency subprocessor is enabled")
		}
		if p.config.Latency.UnansweredQueries {
			p.activeTransforms = append(p.activeTransforms, p.detectEvictedTimeout)
			p.LogInfo(prefixlog + "unanswered queries subprocessor is enabled")
		}
	}

	if p.config.Suspicious.Enable {
		p.activeTransforms = append(p.activeTransforms, p.suspiciousTransform)
		p.LogInfo(prefixlog + "suspicious subprocessor is " + enabled)
	}

	if p.config.Reducer.Enable {
		p.LogInfo(prefixlog + "reducer subprocessor is " + enabled)

		p.ReducerTransform.LoadActiveReducers()
	}

	if p.config.Extract.Enable {
		if p.config.Extract.AddPayload {
			p.activeTransforms = append(p.activeTransforms, p.addBase64Payload)
			p.LogInfo(prefixlog + "extract subprocessor is enabled")
		}
	}

	if p.config.MachineLearning.Enable {
		p.activeTransforms = append(p.activeTransforms, p.machineLearningTransform)
		p.LogInfo(prefixlog + "machinelearning subprocessor is" + enabled)
	}

	if p.config.ATags.Enable {
		p.activeTransforms = append(p.activeTransforms, p.ATagsTransform.AddTags)
		p.LogInfo(prefixlog + "atags subprocessor is enabled")
	}

	return nil
}

func (p *Transforms) InitDNSMessageFormat(dm *dnsutils.DNSMessage) {
	if p.config.Filtering.Enable {
		p.FilteringTransform.InitDNSMessage(dm)
	}

	if p.config.GeoIP.Enable {
		p.GeoipTransform.InitDNSMessage(dm)
	}

	if p.config.Suspicious.Enable {
		p.SuspiciousTransform.InitDNSMessage(dm)
	}

	if p.config.Normalize.Enable {
		if p.config.Normalize.AddTld || p.config.Normalize.AddTldPlusOne {
			p.NormalizeTransform.InitDNSMessage(dm)
		}
	}

	if p.config.Extract.Enable {
		if p.config.Extract.AddPayload {
			p.ExtractProcessor.InitDNSMessage(dm)
		}
	}

	if p.config.Reducer.Enable {
		p.ReducerTransform.InitDNSMessage(dm)
	}

	if p.config.MachineLearning.Enable {
		p.MachineLearningTransform.InitDNSMessage(dm)
	}

	if p.config.ATags.Enable {
		p.ATagsTransform.InitDNSMessage(dm)
	}
}

func (p *Transforms) Reset() {
	if p.config.GeoIP.Enable {
		p.GeoipTransform.Close()
	}
}

func (p *Transforms) LogInfo(msg string, v ...interface{}) {
	p.logger.Info(pkgutils.PrefixLogTransformer+"["+p.name+"] "+msg, v...)
}

func (p *Transforms) LogError(msg string, v ...interface{}) {
	p.logger.Error(pkgutils.PrefixLogTransformer+"["+p.name+"] "+msg, v...)
}

// transform functions: return code
func (p *Transforms) machineLearningTransform(dm *dnsutils.DNSMessage) int {
	p.MachineLearningTransform.AddFeatures(dm)
	return ReturnSuccess
}

func (p *Transforms) suspiciousTransform(dm *dnsutils.DNSMessage) int {
	p.SuspiciousTransform.CheckIfSuspicious(dm)
	return ReturnSuccess
}

func (p *Transforms) geoipTransform(dm *dnsutils.DNSMessage) int {
	geoInfo, err := p.GeoipTransform.Lookup(dm.NetworkInfo.QueryIP)
	if err != nil {
		p.LogError("geoip lookup error %v", err)
		return ReturnError
	}

	dm.Geo.Continent = geoInfo.Continent
	dm.Geo.CountryIsoCode = geoInfo.CountryISOCode
	dm.Geo.City = geoInfo.City
	dm.Geo.AutonomousSystemNumber = geoInfo.ASN
	dm.Geo.AutonomousSystemOrg = geoInfo.ASO

	return ReturnSuccess
}

func (p *Transforms) anonymizeIP(dm *dnsutils.DNSMessage) int {
	dm.NetworkInfo.QueryIP = p.UserPrivacyTransform.AnonymizeIP(dm.NetworkInfo.QueryIP)

	return ReturnSuccess
}

func (p *Transforms) hashIP(dm *dnsutils.DNSMessage) int {
	dm.NetworkInfo.QueryIP = p.UserPrivacyTransform.HashIP(dm.NetworkInfo.QueryIP)
	dm.NetworkInfo.ResponseIP = p.UserPrivacyTransform.HashIP(dm.NetworkInfo.ResponseIP)
	return ReturnSuccess
}

func (p *Transforms) measureLatency(dm *dnsutils.DNSMessage) int {
	p.LatencyTransform.MeasureLatency(dm)
	return ReturnSuccess
}

func (p *Transforms) detectEvictedTimeout(dm *dnsutils.DNSMessage) int {
	p.LatencyTransform.DetectEvictedTimeout(dm)
	return ReturnSuccess
}

func (p *Transforms) minimazeQname(dm *dnsutils.DNSMessage) int {
	dm.DNS.Qname = p.UserPrivacyTransform.MinimazeQname(dm.DNS.Qname)

	return ReturnSuccess
}

func (p *Transforms) addBase64Payload(dm *dnsutils.DNSMessage) int {
	dm.Extracted.Base64Payload = p.ExtractProcessor.AddBase64Payload(dm)
	return ReturnSuccess
}

func (p *Transforms) ProcessMessage(dm *dnsutils.DNSMessage) int {
	// Begin to normalize
	p.NormalizeTransform.ProcessDNSMessage(dm)

	// Traffic filtering ?
	if p.FilteringTransform.CheckIfDrop(dm) {
		return ReturnDrop
	}

	// Traffic reducer ?
	if p.ReducerTransform.ProcessDNSMessage(dm) == ReturnDrop {
		return ReturnDrop
	}

	//  and finally apply other transformation
	var rCode int
	for _, fn := range p.activeTransforms {
		rCode = fn(dm)
		if rCode != ReturnSuccess {
			return rCode
		}
	}

	return ReturnSuccess
}
