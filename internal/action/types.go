package action

// RDSResizeProposal is the typed v1 action proposal for resizing a single
// RDS instance. Field shape mirrors docs/rds_resize.md §2.
type RDSResizeProposal struct {
	DBInstanceIdentifier string          `json:"db_instance_identifier"`
	Region               string          `json:"region"`
	CurrentInstanceClass string          `json:"current_instance_class"`
	TargetInstanceClass  string          `json:"target_instance_class"`
	ApplyImmediately     bool            `json:"apply_immediately"`
	SuccessCriteria      SuccessCriteria `json:"success_criteria"`
	Reasoning            string          `json:"reasoning"`
}

type SuccessCriteria struct {
	Metric             string  `json:"metric"`
	ThresholdPercent   float64 `json:"threshold_percent"`
	VerificationWindow string  `json:"verification_window"`
}
