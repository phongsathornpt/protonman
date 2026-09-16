package todo

// PatchOp enumerates the supported task-plan patch operations.
type PatchOp string

// PatchImpact classifies the blast radius of a patch after validation.
type PatchImpact string

const (
	// PatchImpactUnknown marks an invalid or empty patch.
	PatchImpactUnknown PatchImpact = ""
	// PatchImpactStatusOnly marks a patch that only flips task statuses.
	PatchImpactStatusOnly PatchImpact = "status_only"
	// PatchImpactStructural marks a patch that adds/removes tasks or rewrites text.
	PatchImpactStructural PatchImpact = "structural"
)

const (
	// PatchAdd appends a new task.
	PatchAdd PatchOp = "add"
	// PatchSetStatus flips an existing task's status.
	PatchSetStatus PatchOp = "set_status"
	// PatchSetText rewrites an existing task's text.
	PatchSetText PatchOp = "set_text"
	// PatchRemove deletes an existing task.
	PatchRemove PatchOp = "remove"
)

// Operation is one validated task-plan mutation.
type Operation struct {
	Op     PatchOp `json:"op"`
	ID     string  `json:"id"`
	Text   string  `json:"text,omitempty"`
	Status Status  `json:"status,omitempty"`
}

// PatchEffects summarizes which plan surfaces a validated patch touches.
type PatchEffects struct {
	Structural bool
	Text       bool
	Status     bool
	Valid      bool
}

// EffectsOfPatch validates the operation vocabulary and classifies the effects.
func EffectsOfPatch(operations []Operation) PatchEffects {
	if len(operations) == 0 {
		return PatchEffects{}
	}
	effects := PatchEffects{Valid: true}
	for _, operation := range operations {
		switch operation.Op {
		case PatchAdd, PatchRemove:
			effects.Structural = true
		case PatchSetText:
			effects.Text = true
		case PatchSetStatus:
			effects.Status = true
		default:
			return PatchEffects{}
		}
	}
	return effects
}

// ClassifyPatch maps validated effects onto the coarse patch-impact policy.
func ClassifyPatch(operations []Operation) PatchImpact {
	effects := EffectsOfPatch(operations)
	if !effects.Valid {
		return PatchImpactUnknown
	}
	if effects.Structural || effects.Text {
		return PatchImpactStructural
	}
	if effects.Status {
		return PatchImpactStatusOnly
	}
	return PatchImpactUnknown
}
