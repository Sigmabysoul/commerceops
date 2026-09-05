# Printer agent operations

The printer agent is a hardware bridge, not a second backend. CommerceOps owns
authorization, tenant selection, artifacts, copy limits, and job state. The
agent owns only local discovery, verified download, durable at-most-once
submission, and status reporting.

## Provision and run

1. A user with `printers.manage` creates a Linux agent in Printing
   Administration and copies the one-time credential.
2. Install CUPS client commands (`lpstat` and `lp`) on the computer connected to
   the printers.
3. Build `go build ./cmd/printer-agent` from `services/api`.
4. Configure these process environment values without committing them:
   - `COMMERCEOPS_URL`: HTTPS server origin, with no trailing slash.
   - `PRINTER_AGENT_CREDENTIAL`: the one-time displayed opaque credential.
   - `PRINTER_AGENT_JOURNAL`: persistent journal path; defaults to
     `./printer-agent-journal.jsonl`.
5. Run the agent under the operating system's process supervisor with a private
   working directory and access only to CUPS and the journal.

The journal must be on durable local storage and retained across upgrades. It
contains job IDs and submission states, not PDF content or credentials. PDFs
are downloaded to mode-0600 temporary files and removed after submission.

## Failure semantics

The agent records `submitted` before invoking CUPS. Therefore reconnecting can
never automatically submit the same server job twice. A machine failure between
that record and the CUPS call can leave an ambiguous job; an operator checks the
printer and uses the explicit retry action if needed. This favors a detectable
miss over duplicate physical labels.

Revoking an agent invalidates all its credentials and takes its printers
offline. Disabling one registered printer prevents new claims without changing
the device credential.

## Windows boundary

Only the `PrinterBackend` interface is platform-specific. A future Windows
spooler backend implements discovery and PDF submission behind that interface;
the server protocol, job state machine, journal, and UI remain unchanged.
