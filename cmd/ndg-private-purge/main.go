// ndg-private-purge is a TLS client of the isolated Docker candidate.
// The host utility never opens a project, indexed source, or Worker socket.
package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type options struct {
	endpoint, caPath, tokenPath, mode, action string
	itemID, storage, target, keeper           string
	planID, digest, confirmation              string
}

func (o options) request() (string, map[string]string, error) {
	route, prefix := "/gov/source-direct-purge", "SOURCE_DIRECT_PURGE_"
	switch o.mode {
	case "source":
	case "quarantine":
		route, prefix = "/gov/quarantine-purge", "QUARANTINE_PURGE_"
	default:
		return "", nil, errors.New("select source or quarantine mode")
	}
	p := map[string]string{"op": prefix + strings.ToUpper(o.action)}
	switch o.action {
	case "list":
		if o.mode != "source" {
			return "", nil, errors.New("list is available in source mode")
		}
	case "plan":
		if o.mode == "source" {
			p["storage_id"], p["target_id"], p["keeper_id"] = o.storage, o.target, o.keeper
		} else {
			p["item_id"] = o.itemID
		}
	case "execute":
		p["confirmation"] = o.confirmation
		fallthrough
	case "approve", "dry_run", "cancel", "recover", "receipt":
		p["plan_id"], p["digest"] = o.planID, o.digest
	default:
		return "", nil, errors.New("unknown action")
	}
	for _, value := range p {
		if value == "" {
			return "", nil, errors.New("missing action argument")
		}
	}
	return route, p, nil
}

func run(o options) error {
	route, payload, err := o.request()
	if err != nil {
		return err
	}
	target, err := url.Parse(o.endpoint)
	if err != nil || target.Scheme != "https" || target.Hostname() == "" || (target.Path != "" && target.Path != "/") ||
		target.User != nil || target.RawQuery != "" || target.Fragment != "" {
		return errors.New("endpoint must be a private HTTPS origin")
	}
	target.Path = route
	token, err := os.ReadFile(o.tokenPath)
	if err != nil || !strings.HasPrefix(strings.TrimSpace(string(token)), "ndg-gov-") {
		return errors.New("cannot read the private token file")
	}
	ca, err := os.ReadFile(o.caPath)
	if err != nil {
		return errors.New("cannot read the candidate CA file")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return errors.New("candidate CA is not valid PEM")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequest(http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		return errors.New("cannot construct candidate request")
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	client := &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}},
		Timeout:   5 * time.Minute,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	response, err := client.Do(request)
	if err != nil {
		return errors.New("candidate request failed; query the receipt before retrying an execution")
	}
	defer response.Body.Close()
	if _, err := io.Copy(os.Stdout, io.LimitReader(response.Body, 64<<10)); err != nil {
		return errors.New("cannot read the candidate response")
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("candidate returned HTTP %d", response.StatusCode)
	}
	return nil
}

func main() {
	var o options
	flag.StringVar(&o.endpoint, "url", "https://10.254.140.2:8443", "candidate HTTPS origin")
	flag.StringVar(&o.caPath, "ca-file", "", "candidate TLS CA certificate")
	flag.StringVar(&o.tokenPath, "token-file", "", "private Bearer token file")
	flag.StringVar(&o.mode, "mode", "source", "source or quarantine")
	flag.StringVar(&o.action, "action", "", "list, plan, approve, dry_run, execute, receipt, cancel or recover")
	flag.StringVar(&o.itemID, "item-id", "", "quarantine item ID")
	flag.StringVar(&o.storage, "storage-id", "", "source storage ID")
	flag.StringVar(&o.target, "target-id", "", "source file ID")
	flag.StringVar(&o.keeper, "keeper-id", "", "source keeper file ID")
	flag.StringVar(&o.planID, "plan-id", "", "persisted plan ID")
	flag.StringVar(&o.digest, "digest", "", "persisted approval digest")
	flag.StringVar(&o.confirmation, "confirmation", "", "exact plan confirmation for irreversible execution")
	flag.Parse()
	if err := run(o); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
