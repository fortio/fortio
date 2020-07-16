// Copyright 2015 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package monitoring

import (
	"encoding/hex"

	"flag"

	"github.com/ldemailly/go-flagz"
	"github.com/prometheus/client_golang/prometheus"
)

type flagType string

const (
	DynamicFlags flagType = "dynamic"
	StaticFlags  flagType = "static"
)

// MustRegisterFlagSet adds relevant Prometheus collectors for the checksum of the FlagSet.
// If the FlagSet is already registered for colletion, this code panics.
func MustRegisterFlagSet(flagSetName string, flagSet *flag.FlagSet) {
	prometheus.MustRegister(NewFlagSetCollector(flagSetName, DynamicFlags, flagSet))
	prometheus.MustRegister(NewFlagSetCollector(flagSetName, StaticFlags, flagSet))
}

type flagSetCollector struct {
	typ     flagType
	desc    *prometheus.Desc
	flagSet *flag.FlagSet
}

// NewFlagSetCollector returns a Prometheus collector that computes the FlagSet checksum on every scrape.
func NewFlagSetCollector(name string, typ flagType, flagSet *flag.FlagSet) prometheus.Collector {
	return &flagSetCollector{
		typ: typ,
		desc: prometheus.NewDesc(
			"flagz_checksum",
			"The FNF32 checksum of the provided flagz FlagSet.",
			[]string{"checksum"},
			prometheus.Labels{"set": name, "type": string(typ)},
		),
		flagSet: flagSet,
	}
}

func (cc *flagSetCollector) Describe(c chan<- *prometheus.Desc) {
	c <- cc.desc
}

func (cc *flagSetCollector) Collect(c chan<- prometheus.Metric) {
	var checksum []byte
	if cc.typ == DynamicFlags {
		checksum = flagz.ChecksumFlagSet(cc.flagSet, flagz.IsFlagDynamic)
	} else {
		checksum = flagz.ChecksumFlagSet(cc.flagSet, func(f *flag.Flag) bool { return !flagz.IsFlagDynamic(f) })
	}
	c <- prometheus.MustNewConstMetric(
		cc.desc,
		prometheus.GaugeValue,
		1,
		hex.EncodeToString(checksum),
	)
}
