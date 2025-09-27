package ghb

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/malayanand/ghb/api"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Unmarshaler interface {
	UnmarshalGHB(data any) error
}

type unmarshalOptions struct {
	timeFormat *timeFormat
}

func unmarshalBytes(bytes []byte, msg proto.Message, params map[string]string, options *unmarshalOptions) error {
	value := map[string]any{}
	for k, v := range params {
		if existing, ok := value[k]; ok {
			return fmt.Errorf("parameter conflict: %q already exists with value %v", k, existing)
		}
		value[k] = v
	}
	if bytes != nil {
		err := json.Unmarshal(bytes, &value)
		if err != nil {
			return fmt.Errorf("failed to unmarshal request body: %v", err)
		}
	}
	return unmarshalMessage(msg, value, options)
}

func unmarshalMessage(msg proto.Message, value any, options *unmarshalOptions) error {
	if unmarshalable, ok := msg.ProtoReflect().Interface().(Unmarshaler); ok {
		if err := unmarshalable.UnmarshalGHB(value); err != nil {
			return err
		}
		return nil
	}
	objectValue, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("expected map[string]any, got %T", value)
	}

	keysMap, err := jsonToProtoKeys(msg)
	if err != nil {
		return err
	}
	for key, v := range objectValue {
		reflectedMessage := msg.ProtoReflect()
		// get the key from the keysMap
		// this make sures that all the fields are mapped to a single key.
		protoKey, ok := keysMap[key]
		if !ok {
			return fmt.Errorf("field %s not found", key)
		}

		fd := reflectedMessage.Descriptor().Fields().ByName(protoreflect.Name(protoKey))
		if fd == nil {
			return fmt.Errorf("field descriptor for %s not found", protoKey)
		}
		if fd.IsMap() {
			if err := unmarshalMap(fd, msg, v, options); err != nil {
				return err
			}
			continue
		} else if fd.IsList() {
			if err := unmarshalList(fd, msg, v, options); err != nil {
				return err
			}
			continue
		} else if fd.Kind() == protoreflect.MessageKind {
			message := reflectedMessage.Mutable(fd).Message().Interface()
			switch val := message.(type) {
			case Unmarshaler:
				err := val.UnmarshalGHB(v)
				if err != nil {
					return err
				}
			case *timestamppb.Timestamp:
				timestamp, err := options.timeFormat.unmarshal(v)
				if err != nil {
					return err
				}
				reflectedMessage.Set(fd, protoreflect.ValueOf(timestamp.ProtoReflect()))
			default:
				if err := unmarshalMessage(val, v, options); err != nil {
					return err
				}
			}
		} else {
			if err := unmarshalField(fd, msg, v); err != nil {
				return err
			}
		}
	}
	return nil
}

func unmarshalMap(fd protoreflect.FieldDescriptor, msg proto.Message, value any, options *unmarshalOptions) error {
	mapValue, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("expected map for field %s, got %T", fd.Name(), value)
	}
	mp := msg.ProtoReflect().Mutable(fd).Map()
	valueDesc := fd.MapValue()
	for k, v := range mapValue {
		var val protoreflect.Value
		if valueDesc.IsList() {
			val = mp.NewValue()
			if err := unmarshalList(valueDesc, msg, v, options); err != nil {
				return err
			}
		} else if valueDesc.Kind() == protoreflect.MessageKind {
			val = mp.NewValue()
			if err := unmarshalMessage(val.Message().Interface(), v, options); err != nil {
				return err
			}
		} else {
			if tmp, err := scalarValue(valueDesc, v); err != nil {
				return err
			} else {
				val = tmp
			}
		}
		// TODO: handle support for all types of key values
		keyType := protoreflect.ValueOfString(k).MapKey()
		mp.Set(keyType, val)
	}
	return nil
}

func unmarshalList(fd protoreflect.FieldDescriptor, msg proto.Message, value any, options *unmarshalOptions) error {
	listValue, ok := value.([]any)
	if !ok {
		return fmt.Errorf("expected list for field %s, got %T", fd.Name(), value)
	}
	list := msg.ProtoReflect().Mutable(fd).List()
	for _, v := range listValue {
		if fd.Kind() == protoreflect.MessageKind {
			val := list.AppendMutable()
			if err := unmarshalMessage(val.Message().Interface(), v, options); err != nil {
				return err
			}
		} else {
			if val, err := scalarValue(fd, v); err != nil {
				return err
			} else {
				list.Append(val)
			}
		}
	}
	return nil
}

func unmarshalField(fd protoreflect.FieldDescriptor, msg proto.Message, value any) error {
	scalarVal, err := scalarValue(fd, value)
	if err != nil {
		return err
	}
	msg.ProtoReflect().Set(fd, scalarVal)
	return nil
}

func scalarValue(fd protoreflect.FieldDescriptor, v any) (protoreflect.Value, error) {
	switch fd.Kind() {
	case protoreflect.BoolKind:
		return protoreflect.ValueOfBool(v.(bool)), nil
	case protoreflect.Int32Kind:
		return protoreflect.ValueOfInt32(int32(v.(float64))), nil
	case protoreflect.Sint32Kind:
		return protoreflect.ValueOfInt32(int32(v.(float64))), nil
	case protoreflect.Uint32Kind:
		return protoreflect.ValueOfUint32(uint32(v.(float64))), nil
	case protoreflect.Int64Kind:
		return protoreflect.ValueOfInt64(int64(v.(float64))), nil
	case protoreflect.Sint64Kind:
		return protoreflect.ValueOfInt64(int64(v.(float64))), nil
	case protoreflect.Uint64Kind:
		return protoreflect.ValueOfUint64(uint64(v.(float64))), nil
	case protoreflect.Sfixed32Kind:
		return protoreflect.ValueOfInt32(int32(v.(float64))), nil
	case protoreflect.Fixed32Kind:
		return protoreflect.ValueOfUint32(uint32(v.(float64))), nil
	case protoreflect.Sfixed64Kind:
		return protoreflect.ValueOfInt64(int64(v.(float64))), nil
	case protoreflect.Fixed64Kind:
		return protoreflect.ValueOfUint64(uint64(v.(float64))), nil
	case protoreflect.FloatKind:
		return protoreflect.ValueOfFloat32(float32(v.(float64))), nil
	case protoreflect.DoubleKind:
		return protoreflect.ValueOfFloat64(v.(float64)), nil
	case protoreflect.StringKind:
		return protoreflect.ValueOfString(v.(string)), nil
	case protoreflect.BytesKind:
		return protoreflect.ValueOfBytes([]byte(v.(string))), nil
	default:
		return protoreflect.Value{}, fmt.Errorf("unsupported type: %T", v)
	}
}

func extractURLParams(pattern, path string) (map[string]string, error) {
	patternParts := strings.Split(strings.Trim(pattern, "/"), "/")
	pathParts := strings.Split(strings.Trim(path, "/"), "/")
	params := make(map[string]string)

	for i, patternPart := range patternParts {
		if i >= len(pathParts) {
			break
		}
		if strings.HasPrefix(patternPart, "{") && strings.HasSuffix(patternPart, "}") {
			paramName := patternPart[1 : len(patternPart)-1]
			params[paramName] = pathParts[i]
		} else if patternPart != pathParts[i] {
			return nil, fmt.Errorf("http url pattern %v, and path params %v do not match", patternPart, pathParts[i])
		}
	}
	for key, value := range params {
		tmp, err := url.PathUnescape(value)
		if err != nil {
			return nil, fmt.Errorf("failed to unescape path param %s: %v", key, err)
		}
		params[key] = tmp
	}
	return params, nil
}

func jsonToProtoKeys(msg proto.Message) (map[string]string, error) {
	fields := msg.ProtoReflect().Descriptor().Fields()
	keyMap := make(map[string]string)
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		rule := proto.GetExtension(fd.Options(), api.E_Field).(*api.FieldRule)
		// If no rule is specified, then the field name is the key.
		if rule == nil {
			if _, ok := keyMap[string(fd.Name())]; ok {
				return nil, fmt.Errorf("same field %s is specified twice", fd.Name())
			}
			keyMap[string(fd.Name())] = string(fd.Name())
			continue
		}
		if rule.GetJsonName() != "" {
			if _, ok := keyMap[rule.GetJsonName()]; ok {
				return nil, fmt.Errorf("same field %s is specified twice", rule.GetJsonName())
			}
			keyMap[rule.GetJsonName()] = string(fd.Name())
		}
	}
	return keyMap, nil
}
