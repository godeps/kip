package server

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"

	"github.com/elotl/cloud-instance-provider/pkg/api"
	"github.com/elotl/cloud-instance-provider/pkg/nodeclient"
	"github.com/elotl/cloud-instance-provider/pkg/util"
	"github.com/elotl/wsstream"
	"github.com/golang/glog"
	vkapi "github.com/virtual-kubelet/virtual-kubelet/node/api"
	"github.com/virtual-kubelet/virtual-kubelet/trace"
)

func (p *InstanceProvider) GetContainerLogs(ctx context.Context, namespace, podName, containerName string, opts vkapi.ContainerLogOpts) (io.ReadCloser, error) {
	ctx, span := trace.StartSpan(ctx, "GetContainerLogs")
	defer span.End()
	ctx = addAttributes(ctx, span, namespaceKey, namespace, nameKey, podName, containerNameKey, containerName)
	glog.Infof("GetContainerLogs %+v", opts)
	// Pending PR: https://github.com/virtual-kubelet/virtual-kubelet/pull/806
	// follow := opts.Follow
	follow := false
	podName = util.WithNamespace(namespace, podName)
	node, err := p.GetNodeForRunningPod(podName, "")
	if !follow || err != nil || node == nil || len(node.Status.Addresses) == 0 {
		glog.V(4).Infof("pulling logs for pod %+v", opts)
		return p.getContainerLogs(podName, containerName, opts)
	}
	glog.V(4).Infof("tailing logs for pod %+v", opts)
	return p.tailContainerLogs(node, podName, containerName, opts)
}

func (p *InstanceProvider) getContainerLogs(podName, containerName string, opts vkapi.ContainerLogOpts) (io.ReadCloser, error) {
	lines := opts.Tail
	limit := opts.LimitBytes
	foundLog, err := p.findLog(podName, containerName, lines, limit)
	if err != nil {
		return nil, util.WrapError(
			err, "finding logs for %q/%q", podName, containerName)
	}
	buf := ioutil.NopCloser(bytes.NewReader([]byte(foundLog.Content)))
	return buf, nil
}

func (p *InstanceProvider) tailContainerLogs(node *api.Node, podName, containerName string, opts vkapi.ContainerLogOpts) (io.ReadCloser, error) {
	withMetadata := opts.Timestamps
	logsPath := nodeclient.StreamLogsEndpoint(containerName, withMetadata)
	ws, err := p.ItzoClientFactory.GetWSStream(node.Status.Addresses, logsPath)
	if err != nil {
		return nil, util.WrapError(
			err, "could not get logs client for pod %q", podName)
	}
	logs, err := p.findLog(podName, containerName, opts.Tail, opts.LimitBytes)
	if err != nil {
		return nil, util.WrapError(
			err, "finding logs for %q/%q", podName, containerName)
	}
	return &containerLogs{
		ws:  ws,
		buf: []byte(logs.Content),
	}, nil
}

type containerLogs struct {
	ws  *wsstream.WSStream
	buf []byte
}

func (l *containerLogs) Read(buf []byte) (int, error) {
	glog.V(4).Infof("reading logs from ws stream")
	n := 0
	if len(l.buf) > 0 {
		glog.V(4).Infof("reading %d bytes from buffer", len(l.buf))
		n = copy(buf, l.buf)
		l.buf = l.buf[n:]
		return n, nil
	}
	select {
	case <-l.ws.Closed():
		glog.V(4).Infof("ws stream closed")
		return 0, io.EOF
	case frame := <-l.ws.ReadMsg():
		n, b, err := wsstream.UnpackMessage(frame)
		if err != nil {
			glog.Errorf("reading ws stream: %v", err)
			return 0, err
		}
		glog.V(4).Infof("read %d bytes from ws stream", n)
		n = copy(buf, b)
		l.buf = append(l.buf[:], b[n:]...)
		glog.V(4).Infof("copied %d bytes from ws stream", n)
		return n, nil
	}
}

func (l *containerLogs) Close() error {
	glog.V(4).Infof("closing ws stream")
	return l.ws.CloseAndCleanup()
}