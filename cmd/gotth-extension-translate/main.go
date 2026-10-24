package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gotthboard/gotth-extension-translate/internal/translate"
	translationv1 "github.com/gotthboard/gotth-extension-translate/proto/gotth/translation/v1"
	extensionsv1 "github.com/gotthboard/gotth-sdk/gen/go/gotth/extensions/v1"
	"google.golang.org/grpc"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println(translate.BuildVersion)
		return
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gotth-extension-translate: startup failed")
		os.Exit(1)
	}
}

func run() error {
	// Install signal handling before loading configuration, adopting descriptors,
	// starting goroutines, or creating the host socket.
	signalContext, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()

	runtime, err := translate.LoadRuntimeFromEnv()
	if err != nil {
		return err
	}
	if signalContext.Err() != nil {
		return nil
	}
	backendContext, cancelBackend := context.WithTimeout(signalContext, time.Duration(runtime.Config.BackendTimeoutMS)*time.Millisecond)
	backend, err := translate.NewLocalBackend(backendContext, runtime.BackendFD, runtime.Config.BackendPeerPID, runtime.Config.BackendPeerUID, runtime.Config.BackendPeerGID, runtime.Config.BackendModelIdentity, runtime.BackendToken)
	cancelBackend()
	if err != nil {
		return err
	}
	recorder := translate.NewLogRecorder(os.Stderr)
	service, err := translate.NewService(runtime, backend, recorder)
	if err != nil {
		_ = backend.Close()
		_ = recorder.Close()
		return err
	}
	defer func() { _ = service.Close() }()
	if signalContext.Err() != nil {
		return nil
	}

	previousMask := syscall.Umask(0o177)
	listener, err := net.Listen("unix", runtime.SocketPath)
	syscall.Umask(previousMask)
	if err != nil {
		return err
	}
	defer func() { _ = listener.Close(); _ = os.Remove(runtime.SocketPath) }()
	if err := os.Chmod(runtime.SocketPath, 0o600); err != nil {
		return err
	}
	server := grpc.NewServer(
		grpc.UnaryInterceptor(service.UnaryInterceptor),
		grpc.MaxRecvMsgSize(translate.MaxRequestEnvelopeBytes),
		grpc.MaxSendMsgSize(translate.MaxResponseEnvelopeBytes),
	)
	extensionsv1.RegisterExtensionControlServer(server, service)
	translationv1.RegisterTranslationServer(server, service)
	if signalContext.Err() != nil {
		return nil
	}
	// The connected peer credentials and application proof have succeeded.
	service.SetReady()
	serveResult := make(chan error, 1)
	go func() { serveResult <- server.Serve(listener) }()

	select {
	case err = <-serveResult:
		if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			service.SetFailed()
			return err
		}
		return nil
	case <-signalContext.Done():
		service.SetStopping()
		stopped := make(chan struct{})
		go func() {
			server.GracefulStop()
			close(stopped)
		}()
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case <-stopped:
		case <-timer.C:
			server.Stop()
			<-stopped
		}
		return nil
	}
}
