# Provision a Simple Test Cluster

The Terraform config here can be used to provision a simple test cluster with virtual-kubelet.

## Getting Started

You need:
* an AWS account configured (check that e.g. `aws iam get-user` works)
* Terraform >= 0.12
* aws-cli
* jq

You can then apply your config:

    cp env.tfvars.example myenv.tfvars
    vi myenv.tfvars # You can change settings for your cluster here.
    terraform apply -var-file myenv.tfvars

This will create a new VPC and a one-node Kubernetes cluster in it with virtual-kubelet, and show the public IP address of the node when done:

    [...]
    
    Apply complete! Resources: 13 added, 0 changed, 0 destroyed.
    
    Outputs:
    
    node-ip = 34.201.59.101

The deployed cluster has following components.

![alt text](https://github.com/elotl/cloud-instance-provider/blob/master/vk_kip.jpg "VK + KIP Stack")

You can now ssh into the instance using the username "ubuntu", and the ssh key you set in your environment file. (It takes a a minute or two for the instance to bootstrap). On the instance, you can use kubectl to interact with your new cluster:

    $ kubectl get nodes
    NAME                          STATUS   ROLES    AGE   VERSION
    ip-10-0-26-113.ec2.internal   Ready    master   67s   v1.17.3
    virtual-kubelet               Ready    agent    13s   v1.14.0-vk-v0.0.1-125-g3b2cc98

## Run a Pod via Virtual Kubelet

To have a pod run via virtual-kubelet, make sure you add a toleration and/or node selector for virtual-kubelet:

    spec:
      nodeSelector:
        type: virtual-kubelet
      tolerations:
      - key: virtual-kubelet.io/provider
        operator: Exists

You can also remove the taint from the virtual-kubelet node if you don't want to use tolerations for every pod or deployment.