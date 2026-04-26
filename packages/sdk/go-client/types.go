package novanas

// Resource type definitions for the kinds the storage data plane
// reads. Each carries only the fields the controller actually needs;
// extending the api server's schema doesn't break consumers because
// json unmarshal ignores unknown fields.

type ObjectMeta struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type Pool struct {
	APIVersion string     `json:"apiVersion"`
	Kind       string     `json:"kind"`
	Metadata   ObjectMeta `json:"metadata"`
	Spec       PoolSpec   `json:"spec"`
	Status     PoolStatus `json:"status,omitempty"`
}

type PoolSpec struct {
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// The full Pool spec carries many more fields (raid layout,
	// encryption, replication, etc.); the storage controller only
	// reads NodeSelector + the base capacity hints. Extend as needed.
}

type PoolStatus struct {
	Phase         string      `json:"phase,omitempty"`
	NodeCount     int         `json:"nodeCount,omitempty"`
	TotalCapacity string      `json:"totalCapacity,omitempty"`
	Conditions    []Condition `json:"conditions,omitempty"`
}

type Condition struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	Reason             string `json:"reason,omitempty"`
	Message            string `json:"message,omitempty"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty"`
	LastTransitionTime string `json:"lastTransitionTime,omitempty"`
}

type Disk struct {
	APIVersion string     `json:"apiVersion"`
	Kind       string     `json:"kind"`
	Metadata   ObjectMeta `json:"metadata"`
	Spec       DiskSpec   `json:"spec"`
	Status     DiskStatus `json:"status,omitempty"`
}

type DiskSpec struct {
	NodeName string `json:"nodeName,omitempty"`
	Path     string `json:"path,omitempty"`
}

type DiskStatus struct {
	Phase string `json:"phase,omitempty"`
	Size  string `json:"size,omitempty"`
}
