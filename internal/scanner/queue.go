// File queue.go documents the scanner queue boundary.
package scanner

// Queue intentionally remains small: channels provide the bounded queue used by the scanner.
// The file exists to keep the phase structure stable as scheduling policies evolve.
