// Copyright 2015 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

// Package watcher provides an etcd-backed Watcher for syncing FlagSet state with etcd.

package watcher

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"flag"

	etcd "github.com/coreos/etcd/client"
	"github.com/ldemailly/go-flagz"
	"golang.org/x/net/context"
)

var (
	errNoValue        = fmt.Errorf("no value in Node")
	errFlagNotDynamic = fmt.Errorf("flag is not dynamic")
)

// Watcher syncs updates from etcd into a given FlagSet.
type Watcher struct {
	client    etcd.Client
	etcdKeys  etcd.KeysAPI
	flagSet   *flag.FlagSet
	logger    loggerCompatible
	etcdPath  string
	lastIndex uint64
	watching  bool
	context   context.Context
	cancel    context.CancelFunc
}

// Minimum logger interface needed.
// Default "log" and "logrus" should support these.
type loggerCompatible interface {
	Printf(format string, v ...interface{})
}

// New constructs a new Watcher
func New(set *flag.FlagSet, keysApi etcd.KeysAPI, etcdPath string, logger loggerCompatible) (*Watcher, error) {
	if !strings.HasSuffix(etcdPath, "/") {
		etcdPath = etcdPath + "/"
	}
	u := &Watcher{
		flagSet:   set,
		etcdKeys:  keysApi,
		etcdPath:  etcdPath,
		logger:    logger,
		lastIndex: 0,
		watching:  false,
	}
	u.context, u.cancel = context.WithCancel(context.Background())
	return u, nil
}

// Initialize performs the initial read of etcd and sets all flags (dynamic and static) into FlagSet.
func (u *Watcher) Initialize() error {
	if u.lastIndex != 0 {
		return fmt.Errorf("flagz: already initialized.")
	}
	return u.readAllFlags( /* onlyDynamic */ false)
}

// Start kicks off the go routine that syncs dynamic flags from etcd to FlagSet.
func (u *Watcher) Start() error {
	if u.lastIndex == 0 {
		return fmt.Errorf("flagz: not initialized")
	}
	if u.watching {
		return fmt.Errorf("flagz: already watching")
	}
	u.watching = true
	go u.watchForUpdates()
	return nil
}

// Stops the auto-updating go-routine.
func (u *Watcher) Stop() error {
	if !u.watching {
		return fmt.Errorf("flagz: not watching")
	}
	u.logger.Printf("flagz: stopping")
	u.cancel()
	return nil
}

func (u *Watcher) readAllFlags(onlyDynamic bool) error {
	resp, err := u.etcdKeys.Get(u.context, u.etcdPath, &etcd.GetOptions{Recursive: true, Sort: true})
	if err != nil {
		return err
	}
	u.lastIndex = resp.Index
	errorStrings := []string{}
	for _, node := range resp.Node.Nodes {
		flagName, err := u.nodeToFlagName(node)
		if err != nil {
			u.logger.Printf("flagz: ignoring: %v", err)
			continue
		}
		if err := u.setFlag(flagName, node.Value, onlyDynamic); err != nil && err != errNoValue {
			errorStrings = append(errorStrings, err.Error())
		}
	}
	if len(errorStrings) > 0 {
		return fmt.Errorf("flagz: encountered %d errors while parsing flags from etcd: \n  %v",
			len(errorStrings), strings.Join(errorStrings, "\n"))
	}
	return nil
}

func (u *Watcher) setFlag(flagName string, value string, onlyDynamic bool) error {
	if value == "" {
		return errNoValue
	}
	flag := u.flagSet.Lookup(flagName)
	if flag == nil {
		return fmt.Errorf("flag=%v was not found", flagName)
	}
	if onlyDynamic && !flagz.IsFlagDynamic(flag) {
		return errFlagNotDynamic
	}
	// do not call flag.Value.Set, instead go through flagSet.Set to change "changed" state.
	return u.flagSet.Set(flagName, value)
}

func (u *Watcher) watchForUpdates() error {
	// We need to implement our own watcher because the one in go-etcd doesn't handle errorcode 400 and 401.
	// See https://github.com/coreos/etcd/blob/master/Documentation/errorcode.md
	// And https://coreos.com/etcd/docs/2.0.8/api.html#waiting-for-a-change
	watcher := u.etcdKeys.Watcher(u.etcdPath, &etcd.WatcherOptions{AfterIndex: u.lastIndex, Recursive: true})
	u.logger.Printf("flagz: watcher started")
	for u.watching {
		resp, err := watcher.Next(u.context)
		if etcdErr, ok := err.(etcd.Error); ok && etcdErr.Code == etcd.ErrorCodeEventIndexCleared {
			// Our index is out of the Etcd Log. Reread everything and reset index.
			u.logger.Printf("flagz: handling Etcd Index error by re-reading everything: %v", err)
			time.Sleep(200 * time.Millisecond)
			u.readAllFlags( /* onlyDynamic */ true)
			watcher = u.etcdKeys.Watcher(u.etcdPath, &etcd.WatcherOptions{AfterIndex: u.lastIndex, Recursive: true})
			continue
		} else if clusterErr, ok := err.(*etcd.ClusterError); ok {
			// https://github.com/coreos/etcd/issues/3209
			if len(clusterErr.Errors) > 0 && clusterErr.Errors[0] == context.Canceled {
				// same as context.Cancelled case below.
				break
			}
			u.logger.Printf("flagz: etcd ClusterError. Will retry. %v", clusterErr.Detail())
			time.Sleep(100 * time.Millisecond)
			continue
		} else if err == context.DeadlineExceeded {
			u.logger.Printf("flagz: deadline exceeded which watching for changes, continuing watching")
			continue
		} else if err == context.Canceled {
			break
		} else if err != nil {
			u.logger.Printf("flagz: wicked etcd error. Restarting watching after some time. %v", err)
			// Etcd started dropping watchers, or is re-electing. Give it some time.
			randOffsetMs := int(500 * rand.Float32())
			time.Sleep(1*time.Second + time.Duration(randOffsetMs)*time.Millisecond)
			continue
		}
		u.lastIndex = resp.Node.ModifiedIndex
		flagName, err := u.nodeToFlagName(resp.Node)
		if err != nil {
			u.logger.Printf("flagz: ignoring %v at etcdindex=%v", err, u.lastIndex)
			continue
		}
		err = u.setFlag(flagName, resp.Node.Value /*onlyDynamic*/, true)
		if err == errNoValue {
			u.logger.Printf("flagz: ignoring action=%v on flag=%v at etcdindex=%v", resp.Action, flagName, u.lastIndex)
			continue
		} else if err == errFlagNotDynamic {
			u.logger.Printf("flagz: ignoring updating flag=%v at etcdindex=%v, because of: %v", flagName, u.lastIndex, err)
		} else if err != nil {
			u.logger.Printf("flagz: failed updating flag=%v at etcdindex=%v, because of: %v", flagName, u.lastIndex, err)
			u.rollbackEtcdValue(flagName, resp)
		} else {
			u.logger.Printf("flagz: updated flag=%v to value=%v at etcdindex=%v", flagName, resp.Node.Value, u.lastIndex)
		}
	}
	u.logger.Printf("flagz: watcher exited")
	return nil
}

func (u *Watcher) rollbackEtcdValue(flagName string, resp *etcd.Response) {
	var err error
	if resp.PrevNode != nil {
		// It's just a new value that's wrong, roll back to prevNode value atomically.
		_, err = u.etcdKeys.Set(u.context, resp.Node.Key, resp.PrevNode.Value, &etcd.SetOptions{PrevIndex: u.lastIndex})
	} else {
		_, err = u.etcdKeys.Delete(u.context, resp.Node.Key, &etcd.DeleteOptions{PrevIndex: u.lastIndex})
	}
	if etcdErr, ok := err.(etcd.Error); ok && etcdErr.Code == etcd.ErrorCodeTestFailed {
		// Someone probably rolled it back in the meantime.
		u.logger.Printf("flagz: rolled back flag=%v was changed by someone else. All good.", flagName)
	} else if err != nil {
		u.logger.Printf("flagz: rolling back flagz=%v failed: %v", flagName, err)
	} else {
		u.logger.Printf("flagz: rolled back flagz=%v to correct state. All good.", flagName)
	}
}

func (u *Watcher) nodeToFlagName(node *etcd.Node) (string, error) {
	if node.Dir {
		return "", fmt.Errorf("key '%v' is a directory entry", node.Key)
	}
	if !strings.HasPrefix(node.Key, u.etcdPath) {
		return "", fmt.Errorf("key '%v' doesn't start with etcd path '%v'", node.Key, u.etcdPath)
	}
	truncated := strings.TrimPrefix(node.Key, u.etcdPath)
	if strings.Count(truncated, "/") > 0 {
		return "", fmt.Errorf("key '%v' isn't a direct leaf of etcd path '%v'", node.Key, u.etcdPath)
	}
	return truncated, nil
}
