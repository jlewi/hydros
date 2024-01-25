package gcp

import (
	longrunning "cloud.google.com/go/longrunning/autogen"
	"cloud.google.com/go/longrunning/autogen/longrunningpb"
	"context"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"time"
)

var (
	ErrTimeOut = errors.New("Operation timed out")
)

// WaitForOp waits for an operation to complete. Caller should set the deadline on the context.
// On timeout error is nil and the last operation is returned but Done won't be true.
func WaitForOp(ctx context.Context, client *longrunning.OperationsClient, op *longrunningpb.Operation) (*longrunningpb.Operation, error) {
	// TODO(jeremy): We should get the logger from the context?
	deadline, ok := ctx.Deadline()
	if !ok {
		// Set a default deadline of 10 minutes
		deadline = time.Now().Add(10 * time.Minute)
	}

	log, err := logr.FromContext(ctx)
	if err != nil {
		log = zapr.NewLogger(zap.L())
	}

	pause := 5 * time.Second

	var last *longrunningpb.Operation
	for time.Now().Before(deadline) {
		req := longrunningpb.GetOperationRequest{
			Name: op.GetName(),
		}

		// N.B. We can't just do opClient.WaitForOp because I think that does a server side wait and will timeout
		// when the http/grpc timeout is reahed.
		last, err := client.GetOperation(ctx, &req)

		if err != nil {
			// TODO(jeremy): We should decide if this is a permanent or retryable error
			log.Error(err, "Failed to get operation", "name", op.GetName())

		} else if last.GetDone() {
			return last, nil
		}

		if time.Now().Add(pause).After(deadline) {
			return last, err
		}
		time.Sleep(pause)
		continue

	}

	return last, nil
}
