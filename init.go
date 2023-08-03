// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021 Datadog, Inc.

package dd_sdk_go_testing

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/DataDog/dd-sdk-go-testing/internal/constants"
	"github.com/DataDog/dd-sdk-go-testing/internal/utils"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/ext"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

const (
	spanKind      = "test"
	testFramework = "golang.org/pkg/testing"
)

var (
	repoRegex    = regexp.MustCompile(`(?m)\/([a-zA-Z0-9\\\-_.]*)$`)
	failNowRegex = regexp.MustCompile(`testing\.\(\*common\)\.FailNow`)
	fatalRegex   = regexp.MustCompile(`testing\.\(\*common\)\.Fatal`) // should also catch Fatalf
)

// FinishFunc closes a started span and attaches test status information.
type FinishFunc func()

// Run is a helper function to run a `testing.M` object and gracefully stopping the tracer afterwards
func Run(m *testing.M, opts ...tracer.StartOption) int {
	// Preload all CI and Git tags.
	ensureCITags()

	// Check if DD_SERVICE has been set; otherwise we default to repo name.
	if v := os.Getenv("DD_SERVICE"); v == "" {
		if repoUrl, ok := getFromCITags(constants.GitRepositoryURL); ok {
			matches := repoRegex.FindStringSubmatch(repoUrl)
			if len(matches) > 1 {
				repoUrl = strings.TrimSuffix(matches[1], ".git")
			}
			opts = append(opts, tracer.WithService(repoUrl))
		}
	}

	// Initialize tracer
	tracer.Start(opts...)
	exitFunc := func() {
		fmt.Println("flushing exitfunc")
		tracer.Flush()
		fmt.Println("flushing exitfunc done")
		tracer.Stop()
	}
	defer exitFunc()

	// Handle SIGINT and SIGTERM
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-signals
		exitFunc()
		os.Exit(1)
	}()

	// Execute test suite
	return m.Run()
}

// TB is the minimal interface common to T and B.
type TB interface {
	Cleanup(f func())
	Failed() bool
	Helper()
	Name() string
	Skipped() bool
	FailureMsg() string
}

var (
	_ TB = (*testing.T)(nil)
	_ TB = (*testing.B)(nil)
)

// StartTest returns a new span with the given testing.TB interface and options. It uses
// tracer.StartSpanFromContext function to start the span with automatically detected information.
func StartTest(tb TB, opts ...Option) (context.Context, FinishFunc) {
	tb.Helper()

	opts = append(opts, WithIncrementSkipFrame())
	return StartTestWithContext(context.Background(), tb, opts...)
}

// StartTestWithContext returns a new span with the given testing.TB interface and options. It uses
// tracer.StartSpanFromContext function to start the span with automatically detected information.
func StartTestWithContext(ctx context.Context, tb TB, opts ...Option) (context.Context, FinishFunc) {
	// tb.Helper()

	cfg := new(config)
	defaults(cfg)
	for _, fn := range opts {
		fn(cfg)
	}

	pc, _, _, _ := runtime.Caller(cfg.skip)
	fullSuite, truncSuite, _ := utils.GetPackageAndName(pc, cfg.ignoredTestSuitePrefix)
	name := tb.Name()
	fqn := fmt.Sprintf("%s.%s", fullSuite, name)

	testOpts := []tracer.StartSpanOption{
		tracer.ResourceName(fqn),
		tracer.Tag(constants.TestName, name),
		tracer.Tag(constants.TestSuite, truncSuite),
		tracer.Tag(constants.TestFramework, testFramework),
		tracer.Tag(constants.Origin, constants.CIAppTestOrigin),
	}

	switch tb.(type) {
	case *testing.T:
		testOpts = append(testOpts, tracer.Tag(constants.TestType, constants.TestTypeTest))
	case *testing.B:
		testOpts = append(testOpts, tracer.Tag(constants.TestType, constants.TestTypeBenchmark))
	}

	cfg.spanOpts = append(testOpts, cfg.spanOpts...)
	span, ctx := tracer.StartSpanFromContext(ctx, constants.SpanTypeTest, cfg.spanOpts...)

	fmt.Println("hola top level from 1255")

	cleanup := func() {
		var r interface{} = nil

		if r = recover(); r != nil {
			// Panic handling
			span.SetTag(constants.TestStatus, constants.TestStatusFail)
			span.SetTag(ext.Error, true)
			span.SetTag(ext.ErrorMsg, fmt.Sprint(r))
			span.SetTag(ext.ErrorStack, getStacktrace(2))
			span.SetTag(ext.ErrorType, "panic")
		} else {
			// Normal finalization
			span.SetTag(ext.Error, tb.Failed())

			if tb.Failed() {
				span.SetTag(constants.TestStatus, constants.TestStatusFail)
				stackTrace := getStacktrace(2)
				fmt.Println("hola neighbor")
				fmt.Println(tb.FailureMsg())
				span.SetTag(ext.ErrorMsg, tb.FailureMsg())
				fmt.Println("bye neighbor")

				// we can detect if t.FailNow was called from the stacktrace
				// and we can get an accurate stacktrace for a t.FailNow
				// t.Fail doesn't work for a stacktrace result
				// because execution continues after t.Fail is called
				if isFailNow(stackTrace) {
					span.SetTag(ext.ErrorStack, stackTrace)
					span.SetTag(ext.ErrorType, "FailNow")

					// t.Fatal calls FailNow, so we look for it in the stack
					if isFatal(stackTrace) {
						span.SetTag(ext.ErrorType, "Fatal")
					}
				}

			} else if tb.Skipped() {
				span.SetTag(constants.TestStatus, constants.TestStatusSkip)
			} else {
				span.SetTag(constants.TestStatus, constants.TestStatusPass)
			}
		}

		span.Finish(cfg.finishOpts...)

		if r != nil {

			fmt.Println("flushing")
			tracer.Flush()
			fmt.Println("flushing done")
			tracer.Stop()
			panic(r)
		}
	}

	return ctx, cleanup
}

func getStacktrace(skip int) string {
	pcs := make([]uintptr, 256)
	total := runtime.Callers(skip+1, pcs)
	frames := runtime.CallersFrames(pcs[:total])
	buffer := new(bytes.Buffer)
	for {
		if frame, ok := frames.Next(); ok {
			fmt.Fprintf(buffer, "%s\n\t%s:%d\n", frame.Function, frame.File, frame.Line)
		} else {
			break
		}
	}
	return buffer.String()
}

func isFailNow(stackTrace string) bool {
	return failNowRegex.MatchString(stackTrace)
}

func isFatal(stackTrace string) bool {
	return fatalRegex.MatchString(stackTrace)
}
