package ghb

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/malayanand/ghb/api"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type Unmarshaler interface {
	UnmarshalGHB(data interface{}) error
}

func unmarshalBytes(bytes []byte, msg proto.Message, params map[string]string) error {
	var value map[string]interface{}
	err := json.Unmarshal(bytes, &value)
	if err != nil {
		return fmt.Errorf("failed to unmarshal request body: %v", err)
	}
	for k, v := range params {
		if existing, ok := value[k]; ok {
			return fmt.Errorf("parameter conflict: %q already exists with value %v", k, existing)
		}
		value[k] = v
	}
	return unmarshalMessage(msg, value)
}

func unmarshalMessage(msg proto.Message, value interface{}) error {
	if unmarshalable, ok := msg.ProtoReflect().Interface().(Unmarshaler); ok {
		if err := unmarshalable.UnmarshalGHB(value); err != nil {
			return err
		}
		return nil
	}
	objectValue, ok := value.(map[string]interface{})
	if !ok {
		return fmt.Errorf("expected map[string]interface{}, got %T", value)
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
			if err := unmarshalMap(fd, msg, v); err != nil {
				return err
			}
			continue
		} else if fd.IsList() {
			if err := unmarshalList(fd, msg, v); err != nil {
				return err
			}
			continue
		} else if fd.Kind() == protoreflect.MessageKind {
			val := reflectedMessage.Mutable(fd).Message().Interface()
			if unmarshalable, ok := val.(Unmarshaler); ok {
				if err := unmarshalable.UnmarshalGHB(v); err != nil {
					return err
				}
			} else {
				// Default to unmarshalling the message.
				if err := unmarshalMessage(val, v); err != nil {
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

func unmarshalMap(fd protoreflect.FieldDescriptor, msg proto.Message, value interface{}) error {
	mapValue, ok := value.(map[string]interface{})
	if !ok {
		return fmt.Errorf("expected map for field %s, got %T", fd.Name(), value)
	}
	mp := msg.ProtoReflect().Mutable(fd).Map()
	for k, v := range mapValue {
		val := mp.NewValue()
		if fd.IsList() {
			if err := unmarshalList(fd, msg, v); err != nil {
				return err
			}
		} else if fd.Kind() == protoreflect.MessageKind {
			if err := unmarshalMessage(val.Message().Interface(), v); err != nil {
				return err
			}
		} else {
			if err := unmarshalField(fd, msg, v); err != nil {
				return err
			}
		}
		// TODO: handle support for all types of key values
		keyType := protoreflect.ValueOfString(k).MapKey()
		mp.Set(keyType, val)
	}
	return nil
}

func unmarshalList(fd protoreflect.FieldDescriptor, msg proto.Message, value interface{}) error {
	listValue, ok := value.([]interface{})
	if !ok {
		return fmt.Errorf("expected list for field %s, got %T", fd.Name(), value)
	}
	list := msg.ProtoReflect().Mutable(fd).List()
	for _, v := range listValue {
		val := list.AppendMutable()
		if fd.Kind() == protoreflect.MessageKind {
			if err := unmarshalMessage(val.Message().Interface(), v); err != nil {
				return err
			}
		} else {
			if err := unmarshalField(fd, msg, v); err != nil {
				return err
			}
		}
	}
	return nil
}

func unmarshalField(fd protoreflect.FieldDescriptor, msg proto.Message, value interface{}) error {
	scalarVal, err := scalarValue(fd, value)
	if err != nil {
		return err
	}
	msg.ProtoReflect().Set(fd, scalarVal)
	return nil
}

func scalarValue(fd protoreflect.FieldDescriptor, v interface{}) (protoreflect.Value, error) {
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
			return nil, fmt.Errorf("Http url pattern %v, and path params %v do not match", patternPart, pathParts[i])
		}
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
