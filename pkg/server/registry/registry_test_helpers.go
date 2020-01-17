package registry

import (
	"sync"

	"github.com/docker/libkv"
	"github.com/docker/libkv/store"
	"github.com/docker/libkv/store/boltdb"
	"github.com/elotl/cloud-instance-provider/pkg/api"
	"github.com/elotl/cloud-instance-provider/pkg/api/validation"
	"github.com/elotl/cloud-instance-provider/pkg/etcd"
	"github.com/elotl/cloud-instance-provider/pkg/server/cloud"
	"github.com/elotl/cloud-instance-provider/pkg/server/events"
	"github.com/elotl/cloud-instance-provider/pkg/util"
	"github.com/elotl/cloud-instance-provider/pkg/util/instanceselector"
)

var (
	boltdbInitLock sync.Mutex
)

func combineClosers(a, b func()) func() {
	return func() {
		a()
		b()
	}
}

func CreateKVStore(filename string) etcd.Storer {
	boltdbInitLock.Lock()
	boltdb.Register()
	KVStore, err := libkv.NewStore(
		store.Backend("boltdb"),
		[]string{filename},
		&store.Config{
			Bucket: "milpa",
		})
	boltdbInitLock.Unlock()
	if err != nil {
		panic(err)
	}
	return KVStore
}

func makeEventSystem() (*events.EventSystem, func()) {
	quit := make(chan struct{})
	wg := &sync.WaitGroup{}
	e := events.NewEventSystem(quit, wg)
	closer := func() {
		close(quit)
	}
	return e, closer
}

func makeRegistryComponents() (*events.EventSystem, etcd.Storer, func()) {
	es, closer1 := makeEventSystem()
	tf, closer2 := util.MakeTempFileName("milpatf")
	closer := combineClosers(closer1, closer2)
	KVStore := CreateKVStore(tf)
	return es, KVStore, closer
}

func makeFakeStatefulValidator() *validation.StatefulValidator {
	cs, _ := cloud.NewLinkedAZSubnetStatus(cloud.NewMockClient())
	vpcCIDRs := []string{"172.20.0.0/16"}
	sv := validation.NewStatefulValidator(cs, cloud.ProviderAWS, vpcCIDRs)
	return sv
}

func SetupTestNodeRegistry() (*NodeRegistry, func()) {
	es, KVStore, closer := makeRegistryComponents()
	nodeRegistry := NewNodeRegistry(KVStore, api.VersioningCodec{}, es)
	return nodeRegistry, closer
}

func SetupTestPodRegistry() (*PodRegistry, func()) {
	instanceselector.Setup("aws", "us-east-1", "t2.nano")
	es, KVStore, closer := makeRegistryComponents()
	sv := makeFakeStatefulValidator()
	podRegistry := NewPodRegistry(KVStore, api.VersioningCodec{}, es, sv)
	return podRegistry, closer
}

func SetupTestEventRegistry() (*EventRegistry, func()) {
	es, KVStore, closer := makeRegistryComponents()
	reg := NewEventRegistry(KVStore, api.VersioningCodec{}, es)
	return reg, closer
}

func SetupTestLogRegistry() (*LogRegistry, func()) {
	es, KVStore, closer := makeRegistryComponents()
	reg := NewLogRegistry(KVStore, api.VersioningCodec{}, es)
	return reg, closer
}