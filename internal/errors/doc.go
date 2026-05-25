// Package errors is the project's error construction package. Production code
// in obey-installer must construct errors via [New] or [Wrap] and never use
// fmt.Errorf, so every error carries a stable Code suitable for matching,
// metrics, and machine-readable output.
package errors
