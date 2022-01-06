package systemctl

import (
	"systemgo/system"
	"systemgo/unit"
)

type Daemon interface {
	Start(...string) error
	Stop(...string) error
	Isolate(...string) error
	Restart(...string) error
	Reload(...string) error
	Enable(...string) error
	Disable(...string) error

	Units() []*system.Unit
	Status() (system.Status, error)
	StatusOf(string) (unit.Status, error)
	IsEnabled(string) (unit.Enable, error)
	IsActive(string) (unit.Activation, error)
}
