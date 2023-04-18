package main

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	WattboxHost     = os.Getenv("WATTBOX_HOST")
	WattboxUser     = os.Getenv("WATTBOX_USER")
	WattboxPassword = os.Getenv("WATTBOX_PASSWORD")

	voltageGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "wattbox_voltage",
		},
		[]string{},
	)

	wattsGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "wattbox_watts",
		},
		[]string{"outlet"},
	)

	ampsGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "wattbox_amps",
		},
		[]string{"outlet"},
	)
)

func init() {
	prometheus.MustRegister(voltageGauge)
	prometheus.MustRegister(wattsGauge)
	prometheus.MustRegister(ampsGauge)
}

func fetchWattbox() {
	url := fmt.Sprintf("http://%s/main", WattboxHost)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		panic(err)
	}
	wattRequest := WattboxRequest{
		Username: WattboxUser,
		Password: WattboxPassword,
		Client:   &http.Client{},
		Request:  req,
	}

	resp, err := wattbox(wattRequest)
	if err != nil {
		fmt.Println(wattRequest.Request)
		fmt.Println(resp)
		panic(err)
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	doc.Find(".grid-grey").Each(func(i int, s *goquery.Selection) {
		if i == 0 {
			v := s.Find("div:nth-child(3) span").Text()
			voltage, err := strconv.ParseFloat(v[:len(v)-1], 64)
			if err != nil {
				fmt.Println(err)
			} else {
				voltageGauge.With(prometheus.Labels{}).Set(voltage)
			}
		} else if i == 1 {
			s.Find(".grid-block").Each(func(outletIndex int, s *goquery.Selection) {
				s.Find("p").Each(func(i int, s *goquery.Selection) {
					v := s.Text()
					if i == 0 {
						watts, err := strconv.ParseFloat(v[:len(v)-1], 64)
						if err != nil {
							fmt.Println(err)
						} else {
							wattsGauge.With(prometheus.Labels{
								"outlet": fmt.Sprintf("%d", outletIndex),
							}).Set(watts)
						}
					} else if i == 1 {
						amps, err := strconv.ParseFloat(v[:len(v)-1], 64)
						if err != nil {
							fmt.Println(err)
						} else {
							ampsGauge.With(prometheus.Labels{
								"outlet": fmt.Sprintf("%d", outletIndex),
							}).Set(amps)
						}
					}
				})
			})
		}
	})
}

func pollWattbox(d time.Duration) {
	// Get initial values
	fetchWattbox()

	timer := time.NewTicker(d)
	for {
		<-timer.C
		fetchWattbox()
	}
}

func main() {
	if WattboxHost == "" {
		panic("You must supply the WATTBOX_HOST environment variable.")
	}
	if WattboxUser == "" {
		panic("You must supply the WATTBOX_USER environment variable.")
	}
	if WattboxPassword == "" {
		panic("You must supply the WATTBOX_PASSWORD environment variable.")
	}

	duration := os.Getenv("POLL_DURATION")
	if duration == "" {
		duration = "30s"
	}
	d, err := time.ParseDuration(duration)
	if err != nil {
		panic(err)
	}
	go pollWattbox(d)

	http.Handle("/metrics", promhttp.Handler())
	if err := http.ListenAndServe(":8181", nil); err != nil {
		panic(err)
	}

}

type WattboxRequest struct {
	Username string
	Password string
	Client   *http.Client
	Request  *http.Request
}

func wattbox(req WattboxRequest) (*http.Response, error) {
	resp, err := req.Client.Do(req.Request)
	if err != nil {
		return nil, err
	}

	// If we receive a response with a 401 status and WWW-Authenticate header that
	// didn't include Authorization header in the request, retry request with
	// generated Authorization header.
	if resp.StatusCode == http.StatusUnauthorized && req.Request.Header.Get("Authorization") == "" && resp.Header.Get("WWW-Authenticate") != "" {
		req.Request.Header.Set("Authorization", generateAuthorizationHeader(req, resp.Header.Get("WWW-Authenticate")))
		return wattbox(req)
	}

	return resp, nil
}

func DigestAuthParams(authorization string) map[string]string {
	s := strings.SplitN(authorization, " ", 2)
	if len(s) != 2 || s[0] != "Digest" {
		return nil
	}

	return ParsePairs(s[1])
}

// Lifted from https://code.google.com/p/gorilla/source/browse/http/parser/parser.go
func ParsePairs(value string) map[string]string {
	m := make(map[string]string)
	for _, pair := range ParseList(strings.TrimSpace(value)) {
		switch i := strings.Index(pair, "="); {
		case i < 0:
			// No '=' in pair, treat whole string as a 'key'.
			m[pair] = ""
		case i == len(pair)-1:
			// Malformed pair ('key=' with no value), keep key with empty value.
			m[pair[:i]] = ""
		default:
			v := pair[i+1:]
			if v[0] == '"' && v[len(v)-1] == '"' {
				// Unquote it.
				v = v[1 : len(v)-1]
			}
			m[pair[:i]] = v
		}
	}
	return m
}

func ParseList(value string) []string {
	var list []string
	var escape, quote bool
	b := new(bytes.Buffer)
	for _, r := range value {
		switch {
		case escape:
			b.WriteRune(r)
			escape = false
		case quote:
			if r == '\\' {
				escape = true
			} else {
				if r == '"' {
					quote = false
				}
				b.WriteRune(r)
			}
		case r == ',':
			list = append(list, strings.TrimSpace(b.String()))
			b.Reset()
		case r == '"':
			quote = true
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	// Append last part.
	if s := b.String(); s != "" {
		list = append(list, strings.TrimSpace(s))
	}
	return list
}

func H(data string) string {
	digest := md5.New()
	digest.Write([]byte(data))
	return fmt.Sprintf("%x", digest.Sum(nil))
}

// RandomKey returns a random 16-byte base64 alphabet string
func RandomKey() string {
	k := make([]byte, 12)
	for bytes := 0; bytes < len(k); {
		n, err := rand.Read(k[bytes:])
		if err != nil {
			panic("rand.Read() failed")
		}
		bytes += n
	}
	return base64.StdEncoding.EncodeToString(k)
}

// generateAuthorization generates the Authorization header for the HTTP request to Wattbox using the RFC 2069 digest method.
// See https://en.wikipedia.org/wiki/Digest_access_authentication
func generateAuthorizationHeader(req WattboxRequest, authHeader string) string {
	auth := DigestAuthParams(authHeader)
	cnonce := RandomKey()
	// Use hardcoded first request for random cnonce
	nc := "0000000f"

	ha1 := H(fmt.Sprintf("%s:%s:%s", req.Username, auth["realm"], req.Password))
	ha2 := H(fmt.Sprintf("%s:%s", req.Request.Method, req.Request.URL.Path))
	response := H(fmt.Sprintf("%s:%s:%s:%s:%s:%s", ha1, auth["nonce"], nc, cnonce, auth["qop"], ha2))

	return fmt.Sprintf("Digest username=\"%s\", realm=\"%s\", nonce=\"%s\", uri=\"%s\", response=\"%s\", opaque=\"%s\", qop=%s, nc=%s, cnonce=\"%s\"", req.Username, auth["realm"], auth["nonce"], req.Request.URL.Path, response, auth["opaque"], auth["qop"], nc, cnonce)
}
