package gen

import (
	"fmt"
	"strings"

	"github.com/go-faster/errors"
	"github.com/ogen-go/ogen/gen/ir"
	"github.com/ogen-go/ogen/internal/xmaps"
	"github.com/ogen-go/ogen/jsonschema"
	"go.uber.org/zap"
)

type schemaKey struct {
	jsonschema.Ref
	ir.Encoding
}

func (s schemaKey) String() string {
	return fmt.Sprintf("%s %s", s.Ref, s.Encoding)
}

// tstorage is a type storage.
type tstorage struct {
	refs map[schemaKey]*ir.Type // Key: ref

	// types map contains public types.
	// Public type is any type that has a name:
	//  * Struct
	//  * Alias
	//  * Generic
	//  * Interface
	//  * etc
	//
	// Example:
	// ...
	// requestBody:
	//   content:
	//     application/json:
	//       schema:
	//         type: string <- this type will not present
	//                         in the map because
	//                         the type is anonymous.
	//
	types      map[string]*ir.Type              // Key: type name
	responses  map[jsonschema.Ref]*ir.Response  // Key: ref
	parameters map[jsonschema.Ref]*ir.Parameter // Key: ref

	// wtypes stores references to wrapped types:
	//  * [T]StatusCode
	//  * [T]Headers
	//  * [T]StatusCodeWithHeaders
	wtypes map[[2]jsonschema.Ref]*ir.Type // Key: parent ref + ref

	log *zap.Logger
}

func newTStorage(log *zap.Logger) *tstorage {
	return &tstorage{
		refs:       map[schemaKey]*ir.Type{},
		types:      map[string]*ir.Type{},
		responses:  map[jsonschema.Ref]*ir.Response{},
		parameters: map[jsonschema.Ref]*ir.Parameter{},
		wtypes:     map[[2]jsonschema.Ref]*ir.Type{},
		log:        log,
	}
}

func (s *tstorage) saveType(t *ir.Type) error {
	if !t.Is(ir.KindInterface, ir.KindStruct, ir.KindMap, ir.KindEnum, ir.KindAlias, ir.KindGeneric, ir.KindSum, ir.KindStream) {
		panic(unreachable(t))
	}

	if confT, ok := s.types[t.Name]; ok {
		if t.IsGeneric() {
			// HACK:
			// Currently generator can overwrite same generic type
			// multiple times during IR generation.
			//
			// We need to keep the set of features and methods consistent
			// during this overwrites...
			//
			// Maybe we should instantiate generic types only once when needed
			// and reuse them?
			for _, feature := range confT.Features {
				t.AddFeature(feature)
			}
			for iface := range confT.Implements {
				t.Implement(iface)
			}
		} else {
			return errors.Errorf("schema name conflict: %q", t.Name)
		}
	}

	s.types[t.Name] = t
	return nil
}

func (s *tstorage) checkNameAndRename(key schemaKey, t *ir.Type) (err error) {
	var another *ir.Type
	var ok bool
	if another, ok = s.types[t.Name]; !ok {
		return nil
	}

	// Try to rename the type, with a suffix. Usage should be by reference, so hopefully this won't be an issue.
	oldName := t.Name
	for i := 0; i < 1000; i++ {
		t.Name = fmt.Sprintf("%s_%d", oldName, i)
		if _, ok := s.types[t.Name]; !ok {
			for _i := range t.EnumVariants {
				if strings.HasPrefix(t.EnumVariants[_i].Name, oldName) {
					oldVariantSuffix := t.EnumVariants[_i].Name[len(oldName):]
					t.EnumVariants[_i].Name = t.Name + oldVariantSuffix
				}
			}
			s.log.Info("Renamed type",
				zap.String("old", oldName),
				zap.String("new", t.Name),
			)
			if t.Schema != nil {
				s.log.Debug("  - Schema of the renamed type", zap.String("schema", t.Schema.Ref.String()))
			}
			if another.Schema != nil {
				s.log.Debug("  - Schema of the existing type", zap.String("schema", another.Schema.Ref.String()))
			}
			return nil
		}
	}
	// Reset the name and admit defeat
	t.Name = oldName
	return errors.Errorf("reference %q type name conflict (tried also with _0..1000 as suffix): %q", key, t.Name)
}

func (s *tstorage) saveRef(ref jsonschema.Ref, e ir.Encoding, t *ir.Type) error {
	key := schemaKey{ref, e}
	if _, ok := s.refs[key]; ok {
		return errors.Errorf("reference conflict: %q", key)
	}
	if err := s.checkNameAndRename(key, t); err != nil {
		return err
	}

	s.refs[key] = t
	s.types[t.Name] = t
	return nil
}

func (s *tstorage) saveResponse(ref jsonschema.Ref, r *ir.Response) error {
	if _, ok := s.responses[ref]; ok {
		return errors.Errorf("reference conflict: %q", ref)
	}

	s.responses[ref] = r
	return nil
}

func (s *tstorage) saveWType(parent, ref jsonschema.Ref, t *ir.Type) error {
	key := [2]jsonschema.Ref{parent, ref}
	if _, ok := s.wtypes[key]; ok {
		return errors.Errorf("reference conflict: %q", ref)
	}
	if _, ok := s.types[t.Name]; ok {
		return errors.Errorf("reference %q type name conflict: %q", ref, t.Name)
	}

	s.wtypes[key] = t
	s.types[t.Name] = t
	return nil
}

func (s *tstorage) saveParameter(ref jsonschema.Ref, p *ir.Parameter) error {
	if _, ok := s.parameters[ref]; ok {
		return errors.Errorf("reference conflict: %q", ref)
	}

	s.parameters[ref] = p
	return nil
}

func sameBase(t, tt *ir.Type) bool {
	if t.GenericOf != nil {
		t = t.GenericOf
	}

	if tt.GenericOf != nil {
		tt = tt.GenericOf
	}

	if t.Kind == ir.KindAlias && tt.Kind == ir.KindAlias && t.Name == tt.Name {
		return true
	}

	if t.Kind == ir.KindPrimitive && tt.Kind == ir.KindPrimitive && t.Primitive == tt.Primitive {
		return true
	}

	if t.Kind == tt.Kind && t.Schema != nil && tt.Schema != nil && t.Schema.Type == tt.Schema.Type && t.Name == tt.Name {
		return true
	}

	return false
}

func (s *tstorage) merge(other *tstorage) error {

	// Sort the other.refs to make the merge deterministic
	keys := xmaps.SortedKeysFunc(other.refs, func(i, j schemaKey) int {
		return strings.Compare(i.String(), j.String())
	})

	// Check for merge conflicts.
	for _, ref := range keys {
		t := other.refs[ref]
		if _, ok := s.refs[ref]; ok {
			return errors.Errorf("reference conflict: %q", ref)
		}
		key := schemaKey{ref.Ref, ref.Encoding}
		oldName := t.Name
		if err := s.checkNameAndRename(key, t); err != nil {
			return err
		}
		if oldName != t.Name {
			// Was renamed - so fix the s.types too
			delete(other.types, oldName)
			other.types[t.Name] = t
		}
	}

	for name, t := range other.types {
		if confT, ok := s.types[name]; ok {
			if t.IsGeneric() && sameBase(t, confT) {
				for _, feature := range confT.Features {
					t.AddFeature(feature)
				}
				for iface := range confT.Implements {
					t.Implement(iface)
				}
			} else {
				s.log.Error("Type name conflict",
					zap.String("other.type", fmt.Sprintf("%+v %+v", t, t.GenericOf)),
					zap.String("conflict.type", fmt.Sprintf("%+v %+v", confT, confT.GenericOf)),
				)
				return errors.Errorf("anonymous type name conflict: %q", name)
			}
		}
	}

	for ref := range other.responses {
		if _, ok := s.responses[ref]; ok {
			return errors.Errorf("response reference conflict: %q", ref)
		}
	}

	for ref := range other.wtypes {
		if _, ok := s.wtypes[ref]; ok {
			return errors.Errorf("wrapped type reference conflict: %q", ref)
		}
	}

	for ref := range other.parameters {
		if _, ok := s.parameters[ref]; ok {
			return errors.Errorf("parameter reference conflict: %q", ref)
		}
	}

	// Merge types.
	for ref, t := range other.refs {
		s.refs[ref] = t
		s.types[t.Name] = t
	}

	for name, t := range other.types {
		s.types[name] = t
	}

	for name, t := range other.responses {
		s.responses[name] = t
	}

	for name, t := range other.wtypes {
		s.wtypes[name] = t
	}

	for name, t := range other.parameters {
		s.parameters[name] = t
	}

	return nil
}
