// Package model defines the data shared by parsing, discovery, and routing.
package model

type Account struct {
	Login string
	ID    int64
	Orgs  []string
}

type Target struct {
	Host, Owner, Repo string
}

type Request struct {
	Args           []string
	Passthrough    bool
	CredentialOp   string
	CredentialURL  string
	TokenUser      string
	Fork           bool
	Org            string
	Target         Target
	TargetExplicit bool
	RemoteURL      string
	InheritedToken bool
	RouterToken    bool
}

type DecisionKind string

const (
	Passthrough DecisionKind = "passthrough"
	UseAccount  DecisionKind = "account"
	Probe       DecisionKind = "probe"
	Missing     DecisionKind = "missing"
	Ambiguous   DecisionKind = "ambiguous"
)

type Decision struct {
	Kind    DecisionKind
	Account Account
	Target  Target
	Reason  string
}

type Profile struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}
