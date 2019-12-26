# 使用kubeadm搭建kubernetes v1.16.2记录


# 1. 安装环境
- Kubernetes：v
- Docker CE：19.03.4-3.el7.x86_64
- 创建三台CentOS虚拟主机，一个Master节点，2个Node节点：
  - 系统版本：CentOS Linux release 8.0.1905 (Core)
  - 内核版本：4.18.0-80.el8.x86_64
  - Master角色主机配置为内存8GB，CPU总核心数为2，系统盘空间为50GB
  - Node角色主机配置为内存2GB，CPU总核心数为2，系统盘空间为50GB
  - 主机之间网络互通，并且主机能够访问外网（需要拉取镜像）

总结一下安装条件：

1. 64位Linux操作系统，3.10及以上的内核版本
1. x86或ARM架构
1. 主机之间网络互通，主机能够访问外网
1. Master角色机器至少是2核CPU和2GB内存

# 2. 部署前准备工作
> 每台机器都要做如下初始化环境的操作。


## 配置hosts

```bash
hostnamectl --static set-hostname {{hostname}} # {{hostname}}为当前机器的主机名称

# 为每台服务器添加host解析记录
cat >>/etc/hosts<<EOF
192.168.0.15 kubernetes-0
192.168.0.16 kubernetes-1
192.168.0.17 kubernetes-2
EOF
```

## 关闭selinux和防火墙

```bash
sed -ri 's#(SELINUX=).*#\1disabled#' /etc/selinux/config
setenforce 0
systemctl disable firewalld
systemctl stop firewalld
```

## 关闭虚拟内存
> 为什么关闭虚拟内存？
> - 现有的workloads QoS不支持swap
> - 性能预测需要内存延迟可重复验证，开启swap对预测有影响
> - 会减慢任务执行速度，并且加大磁盘带宽，引入隔离性的问题
> 
参考：
> - [Kubelet needs to allow configuration of container memory-swap](https://github.com/kubernetes/kubernetes/issues/7294)
> - [KUBERNETES, SWAP AND THE VMWARE BALLOON DRIVER](https://frankdenneman.nl/2018/11/15/kubernetes-swap-and-the-vmware-balloon-driver/)

```bash
swapoff -a
sed -i '/ swap / s/^\(.*\)$/#\1/g' /etc/fstab
```

## 配置内核参数

```bash
cat <<EOF > /etc/sysctl.d/k8s.conf
net.bridge.bridge-nf-call-ip6tables = 1
net.bridge.bridge-nf-call-iptables = 1
net.ipv4.ip_forward = 1
vm.swappiness=0
EOF

sysctl --system
```

## 加载IPVS模块

```bash
# 该文件保证节点重启后能自己加载所需模块
cat > /etc/sysconfig/modules/ipvs.modules <<EOF
#!/bin/bash
modprobe -- ip_vs
modprobe -- ip_vs_rr
modprobe -- ip_vs_wrr
modprobe -- ip_vs_sh
modprobe -- nf_conntrack_ipv4
EOF

chmod 755 /etc/sysconfig/modules/ipvs.modules
# 加载模块
bash /etc/sysconfig/modules/ipvs.modules
# 查看加载情况
lsmod | grep -e ip_vs -e nf_conntrack_ipv4
```
加载完模块要保证每个节点都安装了ipset软件包，为了方便查看ipvs的代理规则，再安装管理工具ipvsadm：

```bash
yum install -y ipset ipvsadm
```


## 配置yum源
> 使用阿里源


```bash
cat << EOF > /etc/yum.repos.d/kubernetes.repo
[kubernetes]
[kubernetes]
name=Kubernetes
baseurl=https://mirrors.aliyun.com/kubernetes/yum/repos/kubernetes-el7-x86_64/
enabled=1
gpgcheck=1
repo_gpgcheck=1
gpgkey=https://mirrors.aliyun.com/kubernetes/yum/doc/yum-key.gpg https://mirrors.aliyun.com/kubernetes/yum/doc/rpm-package-key.gpg
EOF

yum makecache
```

## 安装Docker

```bash
sudo yum install -y yum-utils \
device-mapper-persistent-data \
lvm2

sudo yum-config-manager \
  --add-repo \
  https://download.docker.com/linux/centos/docker-ce.repo

sudo dnf install https://download.docker.com/linux/centos/7/x86_64/stable/Packages/containerd.io-1.2.6-3.3.el7.x86_64.rpm

sudo yum install -y docker-ce
systemctl enable docker && systemctl start docker
```

## 安装kubernetes

```bash
sudo yum install -y kubelet kubeadm kubectl
systemctl enable kubelet
```

## 配置Cgroup驱动
官方文档推荐Cgroup驱动使用systemd，理由如下：
> When systemd is chosen as the init system for a Linux distribution, the init process generates and consumes a root control group (`cgroup`) and acts as a cgroup manager. Systemd has a tight integration with cgroups and will allocate cgroups per process. It’s possible to configure your container runtime and the kubelet to use `cgroupfs`. Using `cgroupfs` alongside systemd means that there will then be two different cgroup managers.
> Control groups are used to constrain resources that are allocated to processes. A single cgroup manager will simplify the view of what resources are being allocated and will by default have a more consistent view of the available and in-use resources. When we have two managers we end up with two views of those resources. We have seen cases in the field where nodes that are configured to use `cgroupfs` for the kubelet and Docker, and `systemd` for the rest of the processes running on the node becomes unstable under resource pressure.
> Changing the settings such that your container runtime and kubelet use `systemd` as the cgroup driver stabilized the system. Please note the `native.cgroupdriver=systemd` option in the Docker setup below.


```bash
cat <<EOF > /etc/docker/daemon.json
{
  "exec-opts": ["native.cgroupdriver=systemd"],
  "log-driver": "json-file",
  "log-opts": {
    "max-size": "100m"
  },
  "storage-driver": "overlay2"
}
EOF

systemctl restart docker
# 检查配置是否成功
docker info | grep Cgroup
```

# 3. 部署kubernetes集群

## 3.1. 部署Master节点

### 修改初始化配置文件

```bash
# 打印出默认配置
kubeadm config print init-defaults > kubeadm-init.yaml

# 根据自身环境重新修改配置文件
cat kubeadm-init.yaml
```

```yaml
apiVersion: kubeadm.k8s.io/v1beta2
kind: InitConfiguration
---
apiVersion: kubeadm.k8s.io/v1beta2
certificatesDir: /etc/kubernetes/pki
clusterName: kubernetes
controllerManager: {
  extraArgs: {
    horizontal-pod-autoscaler-use-rest-clients: "true",
    horizontal-pod-autoscaler-sync-period: "10s",
    node-monitor-grace-period: "120s"
  }
}
dns:
  type: CoreDNS
etcd:
  local:
    dataDir: /var/lib/etcd
imageRepository: registry.cn-hangzhou.aliyuncs.com/google_containers
kind: ClusterConfiguration
kubernetesVersion: v1.16.2
scheduler: {}
```

### 初始化集群

```bash
kubeadm init --config kubeadm-init.yaml

...
Your Kubernetes control-plane has initialized successfully!

To start using your cluster, you need to run the following as a regular user:

  mkdir -p $HOME/.kube
  sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
  sudo chown $(id -u):$(id -g) $HOME/.kube/config

You should now deploy a pod network to the cluster.
Run "kubectl apply -f [podnetwork].yaml" with one of the options listed at:
  https://kubernetes.io/docs/concepts/cluster-administration/addons/

Then you can join any number of worker nodes by running the following on each as root:

kubeadm join 192.168.0.15:6443 --token nbbusq.p92jg36mvvatzuhv \
    --discovery-token-ca-cert-hash sha256:fac472c7dce2fd3ce33327c2e0e925a252f43033540ea3096337f9e9ea781e23
```

### 为kubectl命令准备Kubeconfig文件

```bash
mkdir -p $HOME/.kube
sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
sudo chown $(id -u):$(id -g) $HOME/.kube/config
```

### 部署网络插件

```bash
kubectl get nodes

NAME           STATUS      ROLES    AGE     VERSION
kubernetes-0   NotReady    master   2d16h   v1.16.2
```
Master节点NotReady的原因是没有部署网络插件，导致master节点网络没有就绪，网络插件有很多，比如Flannel、Calico等等，这里我们选择Weave：

```bash
kubectl apply -f "https://cloud.weave.works/k8s/net?k8s-version=$(kubectl version | base64 | tr -d '\n')"
```
再次查看节点状态，master节点已变为Ready状态。

## 3.2. 部署Node节点
在其余节点分别执行以下命令：

```bash
kubeadm join 192.168.0.15:6443 --token nbbusq.p92jg36mvvatzuhv \
    --discovery-token-ca-cert-hash sha256:fac472c7dce2fd3ce33327c2e0e925a252f43033540ea3096337f9e9ea781e23
```
部署Node节点不需要`--experimental-control-plane`参数，部署Master集群时，其他Master加入集群需要加上该参数。

## 3.3. 部署容器存储插件
这里选用基于Ceph的存储插件Rook，需要三个配置文件

- [common.yaml](https://raw.githubusercontent.com/rook/rook/release-1.1/cluster/examples/kubernetes/ceph/common.yaml)
- [operator.yaml](https://raw.githubusercontent.com/rook/rook/release-1.1/cluster/examples/kubernetes/ceph/operator.yaml)
- [cluster.yaml](https://raw.githubusercontent.com/rook/rook/release-1.1/cluster/examples/kubernetes/ceph/cluster.yaml)

```bash
kubectl create -f common.yaml
kubectl create -f operator.yaml

# 确认rook-ceph-operator是Running状态再进行下一步操作
kubectl -n rook-ceph get pod

kubectl create -f cluster-test.yaml
```

