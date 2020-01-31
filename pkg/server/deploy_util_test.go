package server

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"io"
	"testing"

	"github.com/elotl/cloud-instance-provider/pkg/api"
	"github.com/elotl/cloud-instance-provider/pkg/nodeclient"
	"github.com/stretchr/testify/assert"
	"github.com/virtual-kubelet/node-cli/manager"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

func tarPkgToPackageFile(tarfile io.Reader) (map[string]packageFile, error) {
	gzr, err := gzip.NewReader(tarfile)
	if err != nil {
		return nil, err
	}
	defer gzr.Close()
	tr := tar.NewReader(gzr)

	tfContents := make(map[string]packageFile)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if header.Typeflag == tar.TypeReg {
			data := make([]byte, header.Size)
			read_so_far := int64(0)
			for read_so_far < header.Size {
				n, err := tr.Read(data[read_so_far:])
				if err == io.EOF {
					break
				}
				if err != nil {
					return nil, err
				}
				read_so_far += int64(n)
			}
			tfContents[header.Name[7:]] = packageFile{
				data: data,
				mode: int32(header.Mode),
			}
		}
	}
	return tfContents, nil
}

func TestMakeDeployPackage(t *testing.T) {
	contents := map[string]packageFile{
		"file1":         packageFile{data: []byte("file1"), mode: 0777},
		"path/to/file2": {data: []byte("file2"), mode: 0400},
	}
	buf, err := makeDeployPackage(contents)
	assert.NoError(t, err)
	tfContents, err := tarPkgToPackageFile(bufio.NewReader(buf))
	assert.NoError(t, err)
	assert.Equal(t, contents, tfContents)
}

func TestGetConfigMapFiles(t *testing.T) {
	trueVal := true
	readonlyVal := int32(0444)
	allPermsVal := int32(0777)
	simpleConfigMap := v1.ConfigMap{
		Data: map[string]string{
			"foo": "foocontent",
			"bar": "barcontent",
		},
		BinaryData: map[string][]byte{
			"zed": []byte("zedstuff"),
		},
	}

	tests := []struct {
		name          string
		vol           api.ConfigMapVolumeSource
		cm            v1.ConfigMap
		isErr         bool
		expectedFiles map[string]packageFile
	}{
		{
			name: "optional is skipped",
			vol: api.ConfigMapVolumeSource{
				Optional: &trueVal,
			},
			cm:            v1.ConfigMap{},
			isErr:         false,
			expectedFiles: map[string]packageFile{},
		},
		{
			name: "no items gets all items, default mode",
			vol: api.ConfigMapVolumeSource{
				Optional: &trueVal,
			},
			cm:    simpleConfigMap,
			isErr: false,
			expectedFiles: map[string]packageFile{
				"foo": packageFile{
					data: []byte("foocontent"),
					mode: defaultVolumeFileMode,
				},
				"bar": packageFile{
					data: []byte("barcontent"),
					mode: defaultVolumeFileMode,
				},
				"zed": packageFile{
					data: []byte("zedstuff"),
					mode: defaultVolumeFileMode,
				},
			},
		},
		{
			name: "only get some items, different default modes",
			vol: api.ConfigMapVolumeSource{
				Optional: &trueVal,
				Items: []api.KeyToPath{
					{
						Key:  "bar",
						Path: "path/to",
						Mode: &allPermsVal,
					},
					{
						Key: "zed",
					},
				},
				DefaultMode: &readonlyVal,
			},
			cm:    simpleConfigMap,
			isErr: false,
			expectedFiles: map[string]packageFile{
				"path/to/bar": packageFile{
					data: []byte("barcontent"),
					mode: allPermsVal,
				},
				"zed": packageFile{
					data: []byte("zedstuff"),
					mode: readonlyVal,
				},
			},
		},
	}
	for _, tc := range tests {
		files, err := getConfigMapFiles(&tc.vol, &tc.cm)
		if tc.isErr {
			assert.Error(t, err, tc.name)
		} else {
			assert.NoError(t, err, tc.name)
			assert.Equal(t, tc.expectedFiles, files, tc.name)
		}
	}
}

func TestDeployVolumes(t *testing.T) {
	trueVal := true
	pod := api.GetFakePod()
	pod.Namespace = "default"
	testNode := api.GetFakeNode()
	tests := []struct {
		name          string
		volumes       []api.Volume
		configMap     *v1.ConfigMap
		secret        *v1.Secret
		expectedFiles map[string]packageFile
		isErr         bool
	}{
		{
			name: "optional packages are skipped",
			volumes: []api.Volume{
				{
					Name: "optional",
					VolumeSource: api.VolumeSource{
						ConfigMap: &api.ConfigMapVolumeSource{
							LocalObjectReference: api.LocalObjectReference{
								Name: "not-present",
							},
							Optional: &trueVal,
						},
					},
				},
			},
			expectedFiles: map[string]packageFile{},
			isErr:         false,
		},
		{
			name: "get configmap, single item",
			volumes: []api.Volume{
				{
					Name: "mytest",
					VolumeSource: api.VolumeSource{
						ConfigMap: &api.ConfigMapVolumeSource{
							LocalObjectReference: api.LocalObjectReference{
								Name: "test-config-map",
							},
							Items: []api.KeyToPath{
								{Key: "bar"},
							},
						},
					},
				},
			},
			configMap: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-config-map",
					Namespace: "default",
				},
				Data: map[string]string{
					"foo": "abc",
					"bar": "123",
				},
			},
			expectedFiles: map[string]packageFile{
				"bar": packageFile{data: []byte("123"), mode: defaultVolumeFileMode},
			},
			isErr: false,
		},
		{
			name: "get secret, single item",
			volumes: []api.Volume{
				{
					Name: "mytest",
					VolumeSource: api.VolumeSource{
						Secret: &api.SecretVolumeSource{
							SecretName: "test-secret",
							Items: []api.KeyToPath{
								{Key: "bar"},
							},
						},
					},
				},
			},
			secret: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"foo": []byte("abc"), // abc -> YWJj
					"bar": []byte("123"), // 123 -> MTIz
				},
			},
			expectedFiles: map[string]packageFile{
				"bar": packageFile{data: []byte("123"), mode: defaultVolumeFileMode},
			},
			isErr: false,
		},
	}
	for _, tc := range tests {
		indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
		if tc.configMap != nil {
			assert.Nil(t, indexer.Add(tc.configMap))
		}
		configMapLister := corev1listers.NewConfigMapLister(indexer)
		if tc.secret != nil {
			assert.Nil(t, indexer.Add(tc.secret))
		}
		secretLister := corev1listers.NewSecretLister(indexer)
		rm, err := manager.NewResourceManager(nil, secretLister, configMapLister, nil)
		if err != nil {
			t.Fatal(err)
		}

		// create the nodeClientFactory
		nc := nodeclient.NewMockItzoClientFactory()
		nc.DeployPackage = func(pod, name string, data io.Reader) error {
			tfContents, err := tarPkgToPackageFile(data)
			assert.NoError(t, err, tc.name)
			assert.Equal(t, tc.expectedFiles, tfContents, tc.name)
			return nil
		}
		pod.Spec.Volumes = tc.volumes
		err = deployPodVolumes(pod, testNode, rm, nc)
		if tc.isErr {
			assert.Error(t, err, tc.name)
		} else {
			assert.NoError(t, err, tc.name)
		}
	}
}
