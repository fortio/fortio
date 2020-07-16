// Copyright 2016 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

// Package kubernetes provides an a K8S ConfigMap watcher for the jobs systems.

package configmap

import (
	"fmt"
	"io/ioutil"
	"path"
	"strings"

	"flag"

	"github.com/fsnotify/fsnotify"
	"github.com/ldemailly/go-flagz"
)

const (
	k8sInternalsPrefix = ".."
	k8sDataSymlink     = "..data"
)

var (
	errFlagNotDynamic = fmt.Errorf("flag is not dynamic")
	errFlagNotFound   = fmt.Errorf("flag not found")
)

// Minimum logger interface needed.
// Default "log" and "logrus" should support these.
type loggerCompatible interface {
	Printf(format string, v ...interface{})
}

type Updater struct {
	started    bool
	dirPath    string
	parentPath string
	watcher    *fsnotify.Watcher
	flagSet    *flag.FlagSet
	logger     loggerCompatible
	done       chan bool
}

func Setup(flagSet *flag.FlagSet, dirPath string, logger loggerCompatible) (*Updater, error) {
	u, err := New(flagSet, dirPath, logger)
	if err != nil {
		return nil, err
	}
	err = u.Initialize()
	if err != nil {
		return nil, err
	}
	if err := u.Start(); err != nil {
		return nil, err
	}
	logger.Printf("Configmap flag value watching initialized on %v", dirPath)
	return u, nil
}

func New(flagSet *flag.FlagSet, dirPath string, logger loggerCompatible) (*Updater, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("flagz: error initializing fsnotify watcher.")
	}
	return &Updater{
		flagSet:    flagSet,
		logger:     logger,
		dirPath:    path.Clean(dirPath),
		parentPath: path.Clean(path.Join(dirPath, "..")), // add parent in case the dirPath is a symlink itself
		watcher:    watcher,
	}, nil
}

func (u *Updater) Initialize() error {
	if u.started {
		return fmt.Errorf("flagz: already initialized updater.")
	}
	return u.readAll( /* allowNonDynamic */ false)
}

// Start kicks off the go routine that watches the directory for updates of values.
func (u *Updater) Start() error {
	if u.started {
		return fmt.Errorf("flagz: updater already started.")
	}
	if err := u.watcher.Add(u.parentPath); err != nil {
		return fmt.Errorf("unable to add parent dir %v to watch: %v", u.parentPath, err)
	}
	if err := u.watcher.Add(u.dirPath); err != nil { // add the dir itself.
		return fmt.Errorf("unable to add config dir %v to watch: %v", u.dirPath, err)
	}
	u.logger.Printf("Now watching %v and %v", u.parentPath, u.dirPath)
	u.done = make(chan bool)
	go u.watchForUpdates()
	return nil
}

// Stops the auto-updating go-routine.
func (u *Updater) Stop() error {
	if !u.started {
		return fmt.Errorf("flagz: not updating")
	}
	u.done <- true
	_ = u.watcher.Remove(u.dirPath)
	_ = u.watcher.Remove(u.parentPath)
	return nil
}

func (u *Updater) readAll(dynamicOnly bool) error {
	files, err := ioutil.ReadDir(u.dirPath)
	if err != nil {
		return fmt.Errorf("flagz: updater initialization: %v", err)
	}
	errorStrings := []string{}
	for _, f := range files {
		if strings.HasPrefix(path.Base(f.Name()), ".") {
			// skip random ConfigMap internals and dot files
			continue
		}
		fullPath := path.Join(u.dirPath, f.Name())
		if err := u.readFlagFile(fullPath, dynamicOnly); err != nil {
			if err == errFlagNotDynamic && dynamicOnly {
				// ignore
			} else {
				errorStrings = append(errorStrings, fmt.Sprintf("flag %v: %v", f.Name(), err.Error()))
			}
		}
	}
	if len(errorStrings) > 0 {
		return fmt.Errorf("encountered %d errors while parsing flags from directory  \n  %v",
			len(errorStrings), strings.Join(errorStrings, "\n"))
	}
	return nil
}

func (u *Updater) readFlagFile(fullPath string, dynamicOnly bool) error {
	flagName := path.Base(fullPath)
	flag := u.flagSet.Lookup(flagName)
	if flag == nil {
		return errFlagNotFound
	}
	if dynamicOnly && !flagz.IsFlagDynamic(flag) {
		return errFlagNotDynamic
	}
	content, err := ioutil.ReadFile(fullPath)
	if err != nil {
		return err
	}
	str := string(content)
	u.logger.Printf("updating %v to %q", flagName, str)
	// do not call flag.Value.Set, instead go through flagSet.Set to change "changed" state.
	return u.flagSet.Set(flagName, str)
}

func (u *Updater) watchForUpdates() {
	u.logger.Printf("starting watching")
	for {
		select {
		case event := <-u.watcher.Events:
			//u.logger.Printf("ConfigMap got fsnotify %v ", event)
			if event.Name == u.dirPath || event.Name == path.Join(u.dirPath, k8sDataSymlink) {
				// case of the whole directory being re-symlinked
				switch event.Op {
				case fsnotify.Create:
					if err := u.watcher.Add(u.dirPath); err != nil { // add the dir itself.
						u.logger.Printf("unable to add config dir %v to watch: %v", u.dirPath, err)
					}
					u.logger.Printf("flagz: Re-reading flags after ConfigMap update.")
					if err := u.readAll( /* dynamicOnly */ true); err != nil {
						u.logger.Printf("flagz: directory reload yielded errors: %v", err.Error())
					}
				case fsnotify.Remove:
				}
			} else if strings.HasPrefix(event.Name, u.dirPath) && !isK8sInternalDirectory(event.Name) {
				//u.logger.Printf("ConfigMap got prefix %v", event)
				switch event.Op {
				case fsnotify.Create, fsnotify.Write, fsnotify.Rename:
					flagName := path.Base(event.Name)
					if err := u.readFlagFile(event.Name, true); err != nil {
						u.logger.Printf("flagz: failed setting flag %s: %v", flagName, err.Error())
					}
				}
			}
		case <-u.done:
			return
		}
	}
}

func isK8sInternalDirectory(filePath string) bool {
	basePath := path.Base(filePath)
	return strings.HasPrefix(basePath, k8sInternalsPrefix)
}
