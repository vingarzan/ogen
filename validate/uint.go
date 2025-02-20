package validate

import "github.com/go-faster/errors"

// Uint validates uint64s.
type Uint struct {
	MultipleOf    uint64
	MultipleOfSet bool

	Min          uint64
	MinSet       bool
	MinExclusive bool

	Max          uint64
	MaxSet       bool
	MaxExclusive bool
}

// SetMultipleOf sets multipleOf validator.
func (t *Uint) SetMultipleOf(v uint64) {
	t.MultipleOfSet = true
	t.MultipleOf = v
}

// SetExclusiveMinimum sets exclusive minimum value.
func (t *Uint) SetExclusiveMinimum(v uint64) {
	t.MinExclusive = true
	t.SetMinimum(v)
}

// SetExclusiveMaximum sets exclusive maximum value.
func (t *Uint) SetExclusiveMaximum(v uint64) {
	t.MaxExclusive = true
	t.SetMaximum(v)
}

// SetMinimum sets minimum value.
func (t *Uint) SetMinimum(v uint64) {
	t.Min = v
	t.MinSet = true
}

// SetMaximum sets maximum value.
func (t *Uint) SetMaximum(v uint64) {
	t.Max = v
	t.MaxSet = true
}

// Set reports whether any validations are set.
func (t Uint) Set() bool {
	return t.MinSet || t.MaxSet || t.MultipleOfSet
}

// Validate returns error if v does not match validation rules.
func (t Uint) Validate(v uint64) error {
	if t.MinSet && (v < t.Min || t.MinExclusive && v == t.Min) {
		return errors.Errorf("value %d less than %d", v, t.Min)
	}
	if t.MaxSet && (v > t.Max || t.MaxExclusive && v == t.Max) {
		return errors.Errorf("value %d greater than %d", v, t.Max)
	}
	if t.MultipleOfSet && (uint64(v)%t.MultipleOf) != 0 {
		return errors.Errorf("value %d is not multiple of %d", v, t.MultipleOf)
	}

	return nil
}
