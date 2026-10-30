// Package httpparam reads chi route parameters as decoded values.
//
// A local slug ID such as "10%25-off" is decoded exactly once, so literal
// percent sequences in stored IDs never double-decode.
package httpparam
