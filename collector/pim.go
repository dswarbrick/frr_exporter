package collector

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	pimSubsystem            = "pim"
	pimNeighborMetrixPrefix = "pim_neighbor"

	pimLabels = []string{"vrf"}

	pimNeighborLabels = append(pimLabels, "neighbor", "iface")
	pimDesc           = map[string]*prometheus.Desc{
		"neighborCount": colPromDesc(pimSubsystem, "neighbors_count_total", "Number of neighbors detected", pimLabels),
		"upTime":        colPromDesc(pimNeighborMetrixPrefix, "uptime_seconds", "How long has the peer ben up.", pimNeighborLabels),
	}
)

func init() {
	registerCollector(pimSubsystem, disabledByDefault, NewPIMCollector)
}

type pimCollector struct {
	logger log.Logger
}

// NewPIMCollector collects PIM metrics, implemented as per the Collector interface.
func NewPIMCollector(logger log.Logger) (Collector, error) {
	return &pimCollector{logger: logger}, nil
}

// Collect implemented as per the Collector interface
func (c *pimCollector) Update(ch chan<- prometheus.Metric) error {

	jsonPIMNeighbors, err := getPIMNeighbors()
	if err != nil {
		return fmt.Errorf("cannot get pim neighbors: %s", err)
	} else {
		if err := processPIMNeighbors(ch, jsonPIMNeighbors, c.logger); err != nil {
			return err
		}
	}
	return nil
}

func getPIMNeighbors() ([]byte, error) {
	args := []string{"-c", "show ip pim vrf all neighbor json"}

	return execVtyshCommand(args...)
}

func processPIMNeighbors(ch chan<- prometheus.Metric, jsonPIMNeighbors []byte, logger log.Logger) error {
	var jsonMap map[string]json.RawMessage
	if err := json.Unmarshal(jsonPIMNeighbors, &jsonMap); err != nil {
		return fmt.Errorf("cannot unmarshal pim neighbors json: %s", err)
	}
	for vrfName, vrfData := range jsonMap {
		neighborCount := 0.0
		var _tempvrfInstance map[string]json.RawMessage
		if err := json.Unmarshal(vrfData, &_tempvrfInstance); err != nil {
			return fmt.Errorf("cannot unmarshal VRF instance json: %s", err)
		}
		for ifaceName, ifaceData := range _tempvrfInstance {
			var neighbors map[string]pimNeighbor
			if err := json.Unmarshal(ifaceData, &neighbors); err != nil {
				return fmt.Errorf("cannot unmarshal neighbor json: %s", err)
			}
			for neighborIp, neighborData := range neighbors {
				neighborCount++
				if uptimeSec, err := parseHMS(neighborData.UpTime); err != nil {
					level.Error(logger).Log("msg", "cannot parse neighbor uptime", "uptime", neighborData.UpTime, "err", err)
				} else {
					// The labels are "vrf", "iface", "neighbor"
					neighborLabels := []string{strings.ToLower(vrfName), strings.ToLower(ifaceName), neighborIp}
					newGauge(ch, pimDesc["upTime"], float64(uptimeSec), neighborLabels...)
				}

			}
		}
		newGauge(ch, pimDesc["neighborCount"], neighborCount, vrfName)
	}
	return nil
}

func parseHMS(st string) (int, error) {
	var h, m, s int
	n, err := fmt.Sscanf(st, "%d:%d:%d", &h, &m, &s)
	if err != nil || n != 3 {
		return 0, err
	}
	return h*3600 + m*60 + s, nil
}

type pimNeighbor struct {
	Interface string
	Neighbor  string
	UpTime    string
}
