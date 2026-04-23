package firewall

import (
	"errors"

	"github.com/teacat99/PortPass/internal/model"
)

// The drivers below are placeholders that satisfy the Driver interface so
// the factory compiles and tests can pick them by name. Real implementations
// land in M4.

// NFTables is a placeholder driver backed by the nft command.
type NFTables struct{}

// NewNFTables constructs an nftables driver (M4 will flesh out the body).
func NewNFTables() *NFTables                          { return &NFTables{} }
func (d *NFTables) Name() string                      { return "nftables" }
func (d *NFTables) HealthCheck() error                { return errNotImplemented("nftables") }
func (d *NFTables) Apply(r *model.Rule) (string, error) {
	return "", errNotImplemented("nftables")
}
func (d *NFTables) Remove(r *model.Rule) error   { return errNotImplemented("nftables") }
func (d *NFTables) List() ([]Applied, error)     { return nil, nil }

// UFW is a placeholder driver backed by the ufw command.
type UFW struct{}

// NewUFW constructs a ufw driver (M4 will flesh out the body).
func NewUFW() *UFW                          { return &UFW{} }
func (d *UFW) Name() string                 { return "ufw" }
func (d *UFW) HealthCheck() error           { return errNotImplemented("ufw") }
func (d *UFW) Apply(r *model.Rule) (string, error) {
	return "", errNotImplemented("ufw")
}
func (d *UFW) Remove(r *model.Rule) error { return errNotImplemented("ufw") }
func (d *UFW) List() ([]Applied, error)   { return nil, nil }

// Firewalld is a placeholder driver backed by firewall-cmd.
type Firewalld struct{}

// NewFirewalld constructs a firewalld driver (M4 will flesh out the body).
func NewFirewalld() *Firewalld              { return &Firewalld{} }
func (d *Firewalld) Name() string           { return "firewalld" }
func (d *Firewalld) HealthCheck() error     { return errNotImplemented("firewalld") }
func (d *Firewalld) Apply(r *model.Rule) (string, error) {
	return "", errNotImplemented("firewalld")
}
func (d *Firewalld) Remove(r *model.Rule) error { return errNotImplemented("firewalld") }
func (d *Firewalld) List() ([]Applied, error)   { return nil, nil }

func errNotImplemented(name string) error {
	return errors.New(name + " driver not implemented yet (planned for M4)")
}
