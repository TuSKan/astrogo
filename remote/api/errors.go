package api

import "errors"

// ErrRetriable marks a failure that was retried and failed anyway.
//
// A Client wraps it around the final *HTTPError whenever its retry policy
// would have retried that status — which, arriving as the final answer, means
// the attempts ran out or were disabled. It is what lets a caller tell a
// transient outage from a request that will never succeed.
//
// It lives here rather than in the parent because this package no longer
// imports it — that is what lets the parent import this one and be the single
// door a caller knocks on. [github.com/TuSKan/astrogo/remote] re-exports it
// under its own name, so an errors.Is against remote.ErrRetriable keeps
// matching and no caller learns this package exists.
var ErrRetriable = errors.New("remote: retriable failure")

// ErrNoResponse marks a request that ended without an answer from the
// service: the host could not be reached, the connection failed, nothing came
// back within the client's timeout, or the caller's own context ended it. A
// status the service did send is an *HTTPError instead, so a caller can tell
// "the service said no" from "the service said nothing".
//
// The parent package reads it to remember a host that does not answer; see
// [github.com/TuSKan/astrogo/remote.ErrUnreachable].
var ErrNoResponse = errors.New("no answer")
