# check-certs
// TODO(user): Add simple overview of use/purpose

## Description
// TODO(user): An in-depth paragraph about your project and overview of use

## Getting Started

```mermaid
graph TD
    subgraph Kubernetes Cluster
        subgraph Node
            A[Application Pod] --> B[Istio Sidecar]
            B --> C[Istio Egress Gateway]
            C --> D[Node Network Interface]
        end
        D -->|SNAT to VIP| E[MetalLB VIP: 172.19.40.50]
    end
    E --> F[External Destination]

    style A fill:#f9f,stroke:#333,stroke-width:2px
    style B fill:#bbf,stroke:#333,stroke-width:2px
    style C fill:#bbf,stroke:#333,stroke-width:2px
    style D fill:#bbf,stroke:#333,stroke-width:2px
    style E fill:#bbf,stroke:#333,stroke-width:2px
    style F fill:#f9f,stroke:#333,stroke-width:2px
```

```mermaid
flowchart TD
 subgraph Rack["Rack"]
        BM1("💻<br>BM Node 1<br>IP: 10.1.1.101")
        BM2("💻<br>BM Node 2<br>IP: 10.1.1.102")
        ToR("↕️<br>ToR Switch<br>GW IP: 10.1.1.1")
  end
 subgraph PostGW["Network Segments (Post-GW)"]
        ControlNet("☁️<br>Control Network")
        ProvNet("☁️<br>Provisioning Network")
  end
 subgraph SIG_VPN["VRF: SIG_VPN"]
        ToR_SIG_VPN("[ToR]<br>GW: 10.1.1.1<br>P2P: 192.168.0.1")
        DCGW_SIG_VPN("[DC-GW]<br>P2P: 192.168.0.2")
  end
 subgraph Control_VPN["VRF: Control VPN"]
        DCGW_Control_VPN("[DC-GW]<br>GW: 172.16.1.1")
  end
 subgraph Prov_VPN["VRF: Prov VPN"]
        DCGW_Prov_VPN("[DC-GW]<br>GW: 172.17.1.1")
  end
 subgraph subGraph5["Data Center"]
        Rack
        DCGW@{ label: "<font size=\"5\">🏛️</font><br><b>Data Center Gateway (DC-GW)</b>" }
        PostGW
        SIG_VPN
        Control_VPN
        Prov_VPN
  end
    BM1 -- VLAN 10 --> ToR
    BM2 -- VLAN 10 --> ToR
    ToR -- "SIG_VPN Link<br>192.168.0.0/30" --> DCGW
    DCGW -- Route Leaking<br><b>(VRF Stitching)</b> --> DCGW_Control_VPN & DCGW_Prov_VPN
    ToR_SIG_VPN <--> DCGW_SIG_VPN
    DCGW -- VLAN 100<br>Control VPN Traffic --> ControlNet
    DCGW -- VLAN 200<br>Prov VPN Traffic --> ProvNet
    Control["Control"]

    DCGW@{ shape: rounded}
    style PostGW fill:#f2f2f2,stroke:#333,stroke-width:2px
    style SIG_VPN fill:#dae8fc,stroke:#6c8ebf,stroke-width:2px,color:#000
    style Prov_VPN fill:#f8cecc,stroke:#b85450,stroke-width:2px,color:#000
    style Control VPN fill:#d5e8d4,stroke:#82b366,stroke-width:2px,color:#000
    linkStyle 2 stroke-width:2px,stroke:blue,stroke-dasharray:5,5,fill:none



```

### Prerequisites
- go version v1.21.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/check-certs:tag
```

**NOTE:** This image ought to be published in the personal registry you specified. 
And it is required to have access to pull the image from the working environment. 
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/check-certs:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin 
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following are the steps to build the installer and distribute this project to users.

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/check-certs:tag
```

NOTE: The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without
its dependencies.

2. Using the installer

Users can just run kubectl apply -f <URL for YAML BUNDLE> to install the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/check-certs/<tag or branch>/dist/install.yaml
```

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

# certchecker
