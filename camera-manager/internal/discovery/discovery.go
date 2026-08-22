package discovery

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"camera-appliance/camera-manager/internal/fingerprint"
	"camera-appliance/camera-manager/internal/redaction"
	"camera-appliance/camera-manager/internal/state"
)

type Options struct {
	Timeout      time.Duration
	LimitPerCIDR int
	Usernames    []string
	Password     string
	IncludeONVIF bool
}

type Subnet struct {
	Interface string `json:"interface"`
	CIDR      string `json:"cidr"`
}

type Result struct {
	Device       state.Device           `json:"device"`
	StreamChecks map[string]StreamProbe `json:"stream_checks"`
	Raw          map[string]any         `json:"raw"`
}

type StreamProbe struct {
	Tested      bool   `json:"tested"`
	Success     bool   `json:"success"`
	URLRedacted string `json:"url_redacted,omitempty"`
	Message     string `json:"message,omitempty"`
	LatencyMS   int64  `json:"latency_ms,omitempty"`
}

type Scanner struct {
	options Options
}

func NewScanner(options Options) *Scanner {
	if options.Timeout <= 0 {
		options.Timeout = 800 * time.Millisecond
	}
	if options.LimitPerCIDR <= 0 {
		options.LimitPerCIDR = 254
	}
	return &Scanner{options: options}
}

func LocalSubnets() ([]Subnet, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var subnets []Subnet
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		name := strings.ToLower(iface.Name)
		if strings.HasPrefix(name, "docker") || strings.HasPrefix(name, "br-") || strings.HasPrefix(name, "veth") || strings.HasPrefix(name, "lo") {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ip, network, err := net.ParseCIDR(addr.String())
			if err != nil || ip.To4() == nil || network == nil {
				continue
			}
			ones, bits := network.Mask.Size()
			if bits != 32 || ones < 24 {
				network.Mask = net.CIDRMask(24, 32)
				network.IP = ip.Mask(network.Mask)
			}
			subnets = append(subnets, Subnet{Interface: iface.Name, CIDR: network.String()})
		}
	}
	sort.Slice(subnets, func(i, j int) bool { return subnets[i].CIDR < subnets[j].CIDR })
	return subnets, nil
}

func (s *Scanner) Scan(ctx context.Context) ([]Result, []Subnet, error) {
	subnets, err := LocalSubnets()
	if err != nil {
		return nil, nil, err
	}
	arp := readARP()
	hosts := candidateHosts(subnets, s.options.LimitPerCIDR)
	if len(hosts) == 0 {
		return []Result{}, subnets, nil
	}
	resultCh := make(chan Result, len(hosts))
	sem := make(chan struct{}, 32)
	var wg sync.WaitGroup
	for _, ip := range hosts {
		select {
		case <-ctx.Done():
			return nil, subnets, ctx.Err()
		default:
		}
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if result, ok := s.probeHost(ctx, ip, arp[ip]); ok {
				resultCh <- result
			}
		}(ip)
	}
	wg.Wait()
	close(resultCh)
	refreshedARP := readARP()
	var results []Result
	for result := range resultCh {
		if result.Device.MACAddress == "" && refreshedARP[result.Device.LastIP] != "" {
			result.Device.MACAddress = refreshedARP[result.Device.LastIP]
			result.Device.ID = fingerprint.DeviceID(result.Device.Fingerprint())
			result.Raw["mac_address"] = result.Device.MACAddress
			rawJSON, _ := json.Marshal(result.Raw)
			result.Device.RawJSON = rawJSON
		}
		results = append(results, result)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Device.LastIP < results[j].Device.LastIP })
	return results, subnets, nil
}

func (s *Scanner) probeHost(ctx context.Context, ip, mac string) (Result, bool) {
	rtspOpen, rtspLatency := tcpOpen(ctx, ip, "554", s.options.Timeout)
	onvifOpen, _ := tcpOpen(ctx, ip, "2020", s.options.Timeout)
	httpSig := httpSignature(ctx, ip, s.options.Timeout)
	if !isDiscoverableCamera(rtspOpen, onvifOpen) {
		return Result{}, false
	}
	hostname := reverseName(ip)
	manufacturer, model := classifyDevice(rtspOpen, onvifOpen, httpSig)
	fp := fingerprint.Normalize(fingerprint.Fingerprint{
		MACAddress:   mac,
		Manufacturer: manufacturer,
		Model:        model,
		Hostname:     hostname,
		LastIP:       ip,
	})
	device := state.Device{
		ID:           fingerprint.DeviceID(fp),
		LastSeenAt:   time.Now().UTC(),
		LastIP:       ip,
		MACAddress:   fp.MACAddress,
		Manufacturer: fp.Manufacturer,
		Model:        fp.Model,
		Hostname:     fp.Hostname,
	}
	checks := map[string]StreamProbe{}
	for _, stream := range []string{"stream1", "stream2"} {
		probe := StreamProbe{Tested: true, Success: rtspOpen, LatencyMS: rtspLatency}
		if rtspOpen {
			probe.Message = "RTSP-Port 554 erreichbar"
			if len(s.options.Usernames) > 0 && s.options.Password != "" {
				rawURL := fmt.Sprintf("rtsp://%s:%s@%s:554/%s", s.options.Usernames[0], s.options.Password, ip, stream)
				probe.URLRedacted = redaction.URL(rawURL)
			} else {
				probe.URLRedacted = fmt.Sprintf("rtsp://%s:554/%s", ip, stream)
				probe.Message = "RTSP-Port erreichbar, Stream-Login nicht getestet"
			}
		} else {
			probe.URLRedacted = fmt.Sprintf("rtsp://%s:554/%s", ip, stream)
			probe.Message = "RTSP-Port 554 nicht erreichbar. Prüfe, ob RTSP/ONVIF in der Kamera aktiv ist."
		}
		checks[stream] = probe
	}
	raw := map[string]any{
		"ip":              ip,
		"mac_address":     fp.MACAddress,
		"rtsp_port_open":  rtspOpen,
		"onvif_port_open": onvifOpen,
		"http_signature":  httpSig,
		"onvif":           "WS-Discovery is not implemented yet; TCP 2020 candidate check active",
	}
	rawJSON, _ := json.Marshal(raw)
	device.RawJSON = rawJSON
	return Result{Device: device, StreamChecks: checks, Raw: raw}, true
}

func candidateHosts(subnets []Subnet, limit int) []string {
	seen := map[string]bool{}
	var hosts []string
	for _, subnet := range subnets {
		ip, network, err := net.ParseCIDR(subnet.CIDR)
		if err != nil || ip.To4() == nil {
			continue
		}
		base := ip.Mask(network.Mask).To4()
		if base == nil {
			continue
		}
		added := 0
		for i := 1; i <= 254 && added < limit; i++ {
			host := net.IPv4(base[0], base[1], base[2], byte(i)).String()
			if !seen[host] {
				seen[host] = true
				hosts = append(hosts, host)
				added++
			}
		}
	}
	return hosts
}

func tcpOpen(ctx context.Context, ip, port string, timeout time.Duration) (bool, int64) {
	start := time.Now()
	conn, err := (&net.Dialer{Timeout: timeout}).DialContext(ctx, "tcp", net.JoinHostPort(ip, port))
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return false, latency
	}
	_ = conn.Close()
	return true, latency
}

func httpSignature(ctx context.Context, ip string, timeout time.Duration) string {
	conn, err := (&net.Dialer{Timeout: timeout}).DialContext(ctx, "tcp", net.JoinHostPort(ip, "80"))
	if err != nil {
		return ""
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	_, _ = fmt.Fprintf(conn, "HEAD / HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", ip)
	data, _ := io.ReadAll(io.LimitReader(conn, 2048))
	return strings.TrimSpace(string(data))
}

func isDiscoverableCamera(rtspOpen, onvifOpen bool) bool {
	return rtspOpen || onvifOpen
}

func isCameraLikeHTTPSignature(signature string) bool {
	lower := strings.ToLower(signature)
	return strings.Contains(lower, "tapo") ||
		strings.Contains(lower, "tp-link") ||
		strings.Contains(lower, "ship 2.0") ||
		strings.Contains(lower, "server: debut")
}

func classifyDevice(rtspOpen, onvifOpen bool, signature string) (string, string) {
	lower := strings.ToLower(signature)
	switch {
	case strings.Contains(lower, "tapo") || strings.Contains(lower, "tp-link") || strings.Contains(lower, "ship 2.0"):
		return "TP-Link", "Tapo Camera Candidate"
	case strings.Contains(lower, "server: debut"):
		return "HTTP", "Camera Candidate"
	case rtspOpen:
		return "RTSP", "Unknown Camera"
	case onvifOpen:
		return "ONVIF", "Camera Candidate"
	default:
		return "Network", "Camera Candidate"
	}
}

func readARP() map[string]string {
	keep := func(ip, mac string) bool {
		return ip != "" && fingerprint.ValidMAC(mac)
	}
	out := map[string]string{}
	if file, err := os.Open("/proc/net/arp"); err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		first := true
		for scanner.Scan() {
			if first {
				first = false
				continue
			}
			fields := strings.Fields(scanner.Text())
			if len(fields) >= 4 {
				mac := fingerprint.Normalize(fingerprint.Fingerprint{MACAddress: fields[3]}).MACAddress
				if keep(fields[0], mac) {
					out[fields[0]] = mac
				}
			}
		}
	}
	if cmdOut, err := exec.Command("ip", "neigh", "show").Output(); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(cmdOut)))
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			for i, field := range fields {
				if field == "lladdr" && i > 0 && i+1 < len(fields) {
					mac := fingerprint.Normalize(fingerprint.Fingerprint{MACAddress: fields[i+1]}).MACAddress
					if keep(fields[0], mac) {
						out[fields[0]] = mac
					}
				}
			}
		}
	}
	if cmdOut, err := exec.Command("arp", "-an").Output(); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(cmdOut)))
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) < 4 || fields[3] == "(incomplete)" || fields[3] == "<incomplete>" {
				continue
			}
			ip := strings.Trim(fields[1], "()")
			mac := fingerprint.Normalize(fingerprint.Fingerprint{MACAddress: fields[3]}).MACAddress
			if keep(ip, mac) {
				out[ip] = mac
			}
		}
	}
	return out
}

func MACForIP(ip string) string {
	return readARP()[ip]
}

func reverseName(ip string) string {
	names, err := net.LookupAddr(ip)
	if err != nil || len(names) == 0 {
		return ""
	}
	return strings.TrimSuffix(names[0], ".")
}

func ErrNoSubnets(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}
