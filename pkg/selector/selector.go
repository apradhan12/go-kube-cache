package selector

// Selector represents a label, field, or namespace selector
type Selector struct {
	Kind     string
	Contents string
}
