package entities

import "github.com/metap-org/metap/pkg/metadata"

func boolPtr(b bool) *bool    { return &b }
func strPtr(s string) *string { return &s }

func field(name, label string, kind metadata.FieldKind, required, indexed, sortable bool) metadata.EntityField {
	f := metadata.EntityField{Name: name, Label: label, Kind: kind}
	if required {
		f.Required = boolPtr(true)
	}
	if indexed {
		f.Indexed = boolPtr(true)
	}
	if sortable {
		f.Sortable = boolPtr(true)
	}
	return f
}

func enumField(name, label string, values []string, required, indexed bool) metadata.EntityField {
	f := field(name, label, metadata.KindEnum, required, indexed, false)
	f.EnumValues = values
	return f
}

func refField(name, label, refEntity, refDisplayField string, required, indexed bool) metadata.EntityField {
	f := field(name, label, metadata.KindReference, required, indexed, false)
	f.RefEntity = strPtr(refEntity)
	f.RefDisplayField = strPtr(refDisplayField)
	return f
}

func transitionNoGuard(action, from, to, label string) metadata.WorkflowTransition {
	return metadata.WorkflowTransition{Action: action, From: from, To: to, Label: label}
}
