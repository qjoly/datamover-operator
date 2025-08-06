package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// DataMover operation counters
	DataMoverOperationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "datamover_operations_total",
			Help: "Total number of DataMover operations by phase and status",
		},
		[]string{"phase", "status", "namespace"},
	)

	// DataMover operation duration
	DataMoverPhaseDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "datamover_phase_duration_seconds",
			Help:    "Duration of DataMover phases in seconds",
			Buckets: prometheus.ExponentialBuckets(1, 2, 10), // 1s to ~17min
		},
		[]string{"phase", "namespace"},
	)

	// Current DataMover states
	DataMoverCurrentPhase = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "datamover_current_phase",
			Help: "Current phase of DataMover operations (0=Initial, 1=CreatingPVC, 2=PVCReady, 3=CreatingPod, 4=Completed, 5=Failed)",
		},
		[]string{"name", "namespace"},
	)

	// PVC cloning metrics
	PVCCloneOperationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "datamover_pvc_clone_operations_total",
			Help: "Total number of PVC clone operations",
		},
		[]string{"status", "namespace"},
	)

	// Pod creation metrics
	PodCreationOperationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "datamover_pod_creation_operations_total",
			Help: "Total number of Pod creation operations",
		},
		[]string{"status", "namespace"},
	)

	// Data sync metrics
	DataSyncOperationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "datamover_data_sync_operations_total",
			Help: "Total number of data sync operations",
		},
		[]string{"status", "namespace"},
	)

	// Error metrics
	DataMoverErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "datamover_errors_total",
			Help: "Total number of errors encountered during DataMover operations",
		},
		[]string{"error_type", "phase", "namespace"},
	)

	// PVC cleanup metrics
	PVCCleanupOperationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "datamover_pvc_cleanup_operations_total",
			Help: "Total number of PVC cleanup operations",
		},
		[]string{"status", "namespace"},
	)
)

// Phase constants for metrics
const (
	PhaseInitialMetric     = 0
	PhaseCreatingPVCMetric = 1
	PhasePVCReadyMetric    = 2
	PhaseCreatingPodMetric = 3
	PhaseCleaningUpMetric  = 4
	PhaseCompletedMetric   = 5
	PhaseFailedMetric      = 6
)

func init() {
	// Register metrics with the controller-runtime metrics registry
	metrics.Registry.MustRegister(
		DataMoverOperationsTotal,
		DataMoverPhaseDuration,
		DataMoverCurrentPhase,
		PVCCloneOperationsTotal,
		PodCreationOperationsTotal,
		DataSyncOperationsTotal,
		DataMoverErrorsTotal,
		PVCCleanupOperationsTotal,
	)
}

// Helper functions to record metrics

func RecordOperationStart(phase, namespace string) {
	DataMoverOperationsTotal.WithLabelValues(phase, "started", namespace).Inc()
}

func RecordOperationSuccess(phase, namespace string) {
	DataMoverOperationsTotal.WithLabelValues(phase, "success", namespace).Inc()
}

func RecordOperationFailure(phase, namespace string) {
	DataMoverOperationsTotal.WithLabelValues(phase, "failure", namespace).Inc()
}

func RecordPhaseDuration(phase, namespace string, duration float64) {
	DataMoverPhaseDuration.WithLabelValues(phase, namespace).Observe(duration)
}

func SetCurrentPhase(name, namespace string, phase float64) {
	DataMoverCurrentPhase.WithLabelValues(name, namespace).Set(phase)
}

func RecordPVCCloneOperation(status, namespace string) {
	PVCCloneOperationsTotal.WithLabelValues(status, namespace).Inc()
}

func RecordPodCreationOperation(status, namespace string) {
	PodCreationOperationsTotal.WithLabelValues(status, namespace).Inc()
}

func RecordDataSyncOperation(status, namespace string) {
	DataSyncOperationsTotal.WithLabelValues(status, namespace).Inc()
}

func RecordError(errorType, phase, namespace string) {
	DataMoverErrorsTotal.WithLabelValues(errorType, phase, namespace).Inc()
}

func RecordPVCCleanupOperation(status, namespace string) {
	PVCCleanupOperationsTotal.WithLabelValues(status, namespace).Inc()
}

func GetPhaseMetricValue(phase string) float64 {
	switch phase {
	case "":
		return PhaseInitialMetric
	case "CreatingClonedPVC":
		return PhaseCreatingPVCMetric
	case "ClonedPVCReady":
		return PhasePVCReadyMetric
	case "CreatingPod":
		return PhaseCreatingPodMetric
	case "CleaningUp":
		return PhaseCleaningUpMetric
	case "Completed":
		return PhaseCompletedMetric
	case "Failed":
		return PhaseFailedMetric
	default:
		return PhaseInitialMetric
	}
}
