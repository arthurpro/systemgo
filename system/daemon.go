package system

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"systemgo/unit"
	"systemgo/unit/service"

	log "github.com/sirupsen/logrus"
)

// Default paths to search for unit paths - Daemon uses those, if none are specified
var DEFAULT_PATHS = []string{"/etc/systemd/system/", "/run/systemd/system", "/lib/systemd/system"}

var supported = map[string]bool{
	".service": true,
	".target":  true,
	".mount":   false,
	".socket":  false,
}

// SupportedSuffix returns a bool indicating if suffix represents a unit type,
// which is supported by Systemgo
func SupportedSuffix(suffix string) bool {
	return supported[suffix]
}

// Supported returns a bool indicating if filename represents a unit type,
// which is supported by Systemgo
func Supported(filename string) bool {
	return SupportedSuffix(filepath.Ext(filename))
}

// Daemon supervises instances of Unit
type Daemon struct {
	// System log
	Log *Log

	// Map of created units (name -> *Unit)
	units map[string]*Unit

	// Paths, where the unit file specifications get searched for
	paths []string

	// System state
	state State

	// System starting time
	since time.Time

	mutex sync.Mutex
}

// New returns an instance of a Daemon ready to use
func New() (sys *Daemon) {
	return &Daemon{
		units: make(map[string]*Unit),

		since: time.Now(),
		Log:   NewLog(),
		paths: DEFAULT_PATHS,
	}
}

// Paths returns paths, which get searched for unit files by sys(first path gets searched first)
func (sys *Daemon) Paths() (paths []string) {
	return sys.paths
}

// SetPaths sets paths, which get searched for unit files by sys(first path gets searched first)
func (sys *Daemon) SetPaths(paths ...string) {
	sys.mutex.Lock()
	defer sys.mutex.Unlock()

	sys.paths = paths
}

// Since returns time, when sys was created
func (sys *Daemon) Since() (t time.Time) {
	return sys.since
}

// IsEnabled returns enable state of the unit held in-memory under specified name.
// If error is returned, it is going to be ErrNotFound
//
// TODO
func (sys *Daemon) IsEnabled(name string) (st unit.Enable, err error) {
	//var u *Unit
	//if u, err = sys.Unit(name); err == nil && sys.Enabled[u] {
	//st = unit.Enabled
	//}
	return -1, ErrNotImplemented
}

// IsActive returns activation state of the unit held in-memory under specified name.
// If error is returned, it is going to be ErrNotFound
func (sys *Daemon) IsActive(name string) (st unit.Activation, err error) {
	var u *Unit
	if u, err = sys.Get(name); err == nil {
		st = u.Active()
	}
	return
}

// StatusOf returns status of the unit held in-memory under specified name.
// If error is returned, it is going to be ErrNotFound
func (sys *Daemon) StatusOf(name string) (st unit.Status, err error) {
	var u *Unit
	if u, err = sys.Get(name); err != nil {
		return
	}

	return u.Status(), nil
}

// Start gets names from internal hashmap, creates a new start transaction and runs it
func (sys *Daemon) Start(names ...string) (err error) {
	log.WithField("names", names).Debugf("sys.Start")

	var tr *transaction
	if tr, err = sys.newTransaction(start, names); err != nil {
		return
	}
	return tr.Run()
}

// Stop gets names from internal hashmap, creates a new stop transaction and runs it
func (sys *Daemon) Stop(names ...string) (err error) {
	log.WithField("names", names).Debugf("sys.Stop")

	var tr *transaction
	if tr, err = sys.newTransaction(stop, names); err != nil {
		return
	}
	return tr.Run()
}

// Isolate gets names from internal hashmap, creates a new start transaction, adds a stop job
// for each unit currently active, but not in the transaction already and runs the transaction
func (sys *Daemon) Isolate(names ...string) (err error) {
	log.WithField("names", names).Debugf("sys.Isolate")

	var tr *transaction
	if tr, err = sys.newTransaction(start, names); err != nil {
		return
	}

	for _, u := range sys.Units() {
		if _, ok := tr.unmerged[u]; ok {
			continue
		}

		if err = tr.add(stop, u, nil, true, true); err != nil {
			return
		}
	}
	return tr.Run()
}

// Restart gets names from internal hashmap, creates a new restart transaction and runs it
func (sys *Daemon) Restart(names ...string) (err error) {
	log.WithField("names", names).Debugf("sys.Restart")

	var tr *transaction
	if tr, err = sys.newTransaction(restart, names); err != nil {
		return
	}
	return tr.Run()
}

// Reload gets names from internal hashmap, creates a new reload transaction and runs it
func (sys *Daemon) Reload(names ...string) (err error) {
	log.WithField("names", names).Debugf("sys.Reload")

	var tr *transaction
	if tr, err = sys.newTransaction(reload, names); err != nil {
		return
	}
	return tr.Run()
}

func (sys *Daemon) newTransaction(typ jobType, names []string) (tr *transaction, err error) {
	sys.mutex.Lock()
	defer sys.mutex.Unlock()

	tr = newTransaction()

	for _, name := range names {
		var dep *Unit
		if dep, err = sys.Get(name); err != nil {
			return nil, err
		}

		if err = tr.add(typ, dep, nil, true, true); err != nil {
			return nil, err
		}
	}
	return
}

// Enable gets names from internal hasmap and calls Enable() on each unit returned
func (sys *Daemon) Enable(names ...string) (err error) {
	log.WithField("names", names).Debugf("sys.Enable")

	return sys.getAndExecute(names, func(u *Unit, gerr error) error {
		if gerr != nil {
			return gerr
		}

		return u.Enable()
	})
}

// Disable gets names from internal hasmap and calls Disable() on each unit returned
func (sys *Daemon) Disable(names ...string) (err error) {
	log.WithField("names", names).Debugf("sys.Disable")

	return sys.getAndExecute(names, func(u *Unit, gerr error) error {
		if gerr != nil {
			return gerr
		}

		return u.Disable()
	})
}

func (sys *Daemon) getAndExecute(names []string, fn func(*Unit, error) error) (err error) {
	for _, name := range names {
		if err = fn(sys.Get(name)); err != nil {
			return
		}
	}
	return
}

// Units returns a slice of all units created
func (sys *Daemon) Units() (units []*Unit) {
	log.Debugf("sys.Units")

	unitSet := map[*Unit]struct{}{}
	for _, u := range sys.units {
		unitSet[u] = struct{}{}
	}

	units = make([]*Unit, 0, len(unitSet))
	for u := range unitSet {
		units = append(units, u)
	}
	return
}

// Unit looks up unit name in the internal hasmap and returns the unit created associated with it
// or nil and ErrNotFound, if it does not exist
func (sys *Daemon) Unit(name string) (u *Unit, err error) {
	log.WithField("name", name).Debug("sys.Unit")

	var ok bool
	if u, ok = sys.units[name]; !ok {
		return nil, ErrNotFound
	}
	return
}

// Get looks up the unit name in the internal hasmap of loaded units and calls
// sys.Load(name) if it can not be found.
// If error is returned, it will be error from sys.Load(name)
func (sys *Daemon) Get(name string) (u *Unit, err error) {
	log.WithField("name", name).Debug("sys.Get")

	if u, err = sys.Unit(name); err != nil || !u.IsLoaded() {
		return sys.load(name)
	}
	return
}

// Supervise creates a *Unit wrapping v and stores it in internal hashmap.
// If a unit with name specified already exists - nil and ErrExists are returned
func (sys *Daemon) Supervise(name string, v unit.Interface) (u *Unit, err error) {
	log.WithFields(log.Fields{
		"name":      name,
		"interface": v,
	}).Debugf("sys.Supervise")

	if u, err = sys.Unit(name); err == nil {
		return nil, ErrExists
	}

	return sys.newUnit(name, v), nil
}

func (sys *Daemon) newUnit(name string, v unit.Interface) (u *Unit) {
	log.WithFields(log.Fields{
		"name":      name,
		"interface": v,
	}).Debugf("sys.newUnit")

	u = NewUnit(v)
	u.name = name

	u.System = sys

	sys.units[name] = u
	if strings.HasSuffix(name, ".service") {
		sys.units[strings.TrimSuffix(name, ".service")] = u
	}

	return
}

// load searches for name in configured paths, parses it, and either overwrites the definition of already
// created Unit or creates a new one
func (sys *Daemon) load(name string) (u *Unit, err error) {
	log.WithField("name", name).Debugln("sys.Load")

	if !Supported(name) {
		return nil, ErrUnknownType
	}

	var paths []string
	if filepath.IsAbs(name) {
		paths = []string{name}
	} else {
		paths = make([]string, len(sys.paths))
		for i, path := range sys.paths {
			paths[i] = filepath.Join(path, name)
		}
	}

	for _, path := range paths {
		var file *os.File
		if file, err = os.Open(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		// Commented out because of gopherjs bug,
		// which breaks systemgo on Browsix
		// See https://goo.gl/AycBTv
		//
		//defer file.Close()

		// Check if a unit for name had already been created
		if u, err = sys.Unit(name); err != nil {
			// If not - create a new one
			var v unit.Interface
			switch filepath.Ext(name) {
			case ".target":
				v = &Target{System: sys}
			case ".service":
				v = &service.Unit{}
			default:
				panic("Trying to load an unsupported unit type")
			}

			u = sys.newUnit(name, v)
		}

		u.path = path
		sys.units[path] = u

		var info os.FileInfo
		if info, err = file.Stat(); err == nil && info.IsDir() {
			err = ErrIsDir
		}
		if err != nil {
			u.Log.Errorf("%s", err)
			file.Close()
			return u, err
		}

		if err = u.Interface.Define(file); err != nil {
			if me, ok := err.(unit.MultiError); ok {
				u.Log.Error("Definition is invalid:")
				for _, errmsg := range me.Errors() {
					u.Log.Error(errmsg)
				}
			} else {
				u.Log.Errorf("Error parsing definition: %s", err)
			}
			u.load = unit.Error
			file.Close()
			return u, err
		}

		u.load = unit.Loaded
		return u, file.Close()
	}

	return nil, ErrNotFound
}

// pathset returns a slice of paths to definitions of supported unit types found in path specified
func pathset(path string) (definitions []string, err error) {
	var file *os.File
	if file, err = os.Open(path); err != nil {
		return nil, err
	}
	defer file.Close()

	var info os.FileInfo
	if info, err = file.Stat(); err != nil {
		return nil, err
	} else if !info.IsDir() {
		return nil, ErrNotDir
	}

	var names []string
	if names, err = file.Readdirnames(0); err != nil {
		return nil, err
	}

	definitions = make([]string, 0, len(names))
	for _, name := range names {
		if Supported(name) {
			definitions = append(definitions, filepath.Join(path, name))
		}
	}

	return
}
