package credential

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/rashidgg/ghroute/internal/model"
	"github.com/rashidgg/ghroute/internal/parse"
)

type Record struct {
	Values map[string]string
	Lines  []string
}

func Read(r io.Reader) (Record, error) {
	record := Record{Values: map[string]string{}}
	s := bufio.NewScanner(r)
	for s.Scan() {
		line := s.Text()
		if line == "" {
			break
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || key == "" {
			return record, fmt.Errorf("invalid git credential record")
		}
		record.Values[key] = value
		record.Lines = append(record.Lines, line)
	}
	if s.Err() != nil {
		return record, fmt.Errorf("cannot read git credential record")
	}
	return record, nil
}

// Replay always includes the terminating blank line required by git's protocol.
func (r Record) Replay() string { return strings.Join(r.Lines, "\n") + "\n\n" }

func (r Record) URL() string {
	if u := r.Values["url"]; u != "" {
		return u
	}
	protocol := r.Values["protocol"]
	if protocol == "" {
		protocol = "https"
	}
	return protocol + "://" + r.Values["host"] + "/" + strings.TrimLeft(r.Values["path"], "/")
}

func (r Record) Target() model.Target { return parse.Target(r.URL()) }

func Write(w io.Writer, token string) error {
	if token == "" || strings.ContainsAny(token, "\r\n") {
		return fmt.Errorf("invalid credential token")
	}
	_, err := fmt.Fprintf(w, "username=x-access-token\npassword=%s\n\n", token)
	return err
}
