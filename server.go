package ghb

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"path"
	"reflect"
	"slices"
	"sync"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"

	"github.com/malayanand/ghb/api"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

type Server struct {
	registerProtoOnce sync.Once
	registerProtoErr  error
	services          map[string]*serviceInfo
	mux               *http.ServeMux

	timeFormat *timeFormat
}

type ServerOption func(*Server)

type serviceInfo struct {
	impl    any
	methods map[string]*grpc.MethodDesc
}

var (
	registerServiceOnce sync.Once
	registerServiceErr  error
)

func NewServer(opts ...ServerOption) *Server {
	server := &Server{
		services: make(map[string]*serviceInfo),
		mux:      http.NewServeMux(),
	}
	options := slices.Concat(defaultServerOptions(), opts)
	for _, opt := range options {
		opt(server)
	}
	return server
}

func defaultServerOptions() []ServerOption {
	return []ServerOption{
		WithTimeFormat(ISOTimeFormat),
	}
}

func WithTimeFormat(format *timeFormat) ServerOption {
	return func(s *Server) {
		s.timeFormat = format
	}
}

func (s *Server) RegisterService(serviceDesc *grpc.ServiceDesc, impl any) {
	expected := reflect.TypeOf(serviceDesc.HandlerType).Elem()
	actual := reflect.TypeOf(impl)
	if !actual.Implements(expected) {
		log.Panicf("impl does not implement serviceDesc.HandlerType")
	}
	s.register(serviceDesc, impl)
}

func (s *Server) register(serviceDesc *grpc.ServiceDesc, impl any) {
	info := &serviceInfo{
		impl:    impl,
		methods: make(map[string]*grpc.MethodDesc),
	}
	for _, method := range serviceDesc.Methods {
		info.methods[method.MethodName] = &method
	}
	s.services[serviceDesc.ServiceName] = info
}

func (s *Server) Serve(lis net.Listener) error {
	if err := s.registerProtosOnce(); err != nil {
		return err
	}
	return http.Serve(lis, s.mux)
}

func (s *Server) registerProtosOnce() error {
	s.registerProtoOnce.Do(func() {
		s.registerProtoErr = s.doRegisterProtos()
	})
	return s.registerProtoErr
}

func (s *Server) doRegisterProtos() error {
	var err error
	protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		for i := 0; i < fd.Services().Len(); i++ {
			service := fd.Services().Get(i)
			if err = s.registerService(service); err != nil {
				return false
			}
		}
		return true
	})
	return err
}

func (s *Server) registerService(service protoreflect.ServiceDescriptor) error {
	methods := service.Methods()
	for i := 0; i < methods.Len(); i++ {
		method := methods.Get(i)
		rule := proto.GetExtension(method.Options(), api.E_Http)
		if rule == nil {
			return nil
		}
		httpRule, ok := rule.(*api.HttpRule)
		if !ok || httpRule == nil {
			return nil
		}
		serviceInfo, ok := s.services[string(service.FullName())]
		if !ok || serviceInfo == nil {
			return fmt.Errorf("service %s not found", service.FullName())
		}
		methodDesc, ok := serviceInfo.methods[string(method.Name())]
		if !ok || methodDesc == nil {
			return fmt.Errorf("method %s not found", method.Name())
		}
		s.handleHttpRule(serviceInfo.impl, httpRule, methodDesc.Handler)
	}
	return nil
}

func (s *Server) handleHttpRule(impl any, httpRule *api.HttpRule, methodHandler grpc.MethodHandler) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		if path == "" {
			path = r.URL.Path
		}
		params, err := extractURLParams(httpRule.Path, path)
		if err != nil {
			badRequest(w, err)
			return
		}

		// TODO: build on what to pass in context.
		ctx := buildContext(context.Background(), r)
		dec := func(in any) error {
			msg, ok := in.(proto.Message)
			if !ok {
				return fmt.Errorf("unsported type: %T", in)
			}
			var body []byte
			var err error
			if r.Body != nil && r.ContentLength != 0 {
				body, err = io.ReadAll(r.Body)
				if err != nil {
					return err
				}
			}
			err = unmarshalBytes(body, msg, params, &unmarshalOptions{
				timeFormat: s.timeFormat,
			})
			if err != nil {
				return fmt.Errorf("failed to unmarshal request body: %v", err)
			}
			return nil
		}

		res, err := methodHandler(impl, ctx, dec, nil)
		if err != nil {
			internalServerError(w, err)
			return
		}
		body, err := marshalBytes(res, &marshalOptions{
			timeFormat: s.timeFormat,
		})
		if err != nil {
			internalServerError(w, err)
			return
		}
		_, err = w.Write(body)
		if err != nil {
			internalServerError(w, err)
			return
		}
	}
	pattern := fmt.Sprintf("%s %s", httpRule.Method.String(), path.Join("/", httpRule.Path))
	s.mux.HandleFunc(pattern, handler)
}

func buildContext(ctx context.Context, r *http.Request) context.Context {
	md := metadata.New(make(map[string]string, len(r.Header)))
	for key, value := range r.Header {
		md.Set(key, value...)
	}
	return metadata.NewIncomingContext(ctx, md)
}
