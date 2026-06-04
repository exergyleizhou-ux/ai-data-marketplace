package compute

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// --- Intel TDX hardware attester (Direction B 阶段2, TEE-node half) ---
//
// tdxAttester produces a REAL remote-attestation quote by reading a TD report
// from the Linux TDX guest device (/dev/tdx_guest) inside a confidential VM. The
// report binds the platform measurement to our (measurement|job|output) tuple via
// the 64-byte REPORTDATA field. The resulting quote is verified out-of-process by
// the KBS/DCAP at key-release time (see remoteKBS) — that, not this process, is
// the cryptographic trust boundary.
//
// HONEST SCOPE: only the OFF-HARDWARE behaviour is unit-tested here — without
// /dev/tdx_guest the attester FAILS CLOSED (ErrTEEUnavailable), so a runner
// mis-configured for TEE can never silently run without real hardware. The ioctl
// success path runs only inside a TDX guest and MUST be validated on a TDX node
// (see docs/部署-L2-TEE节点与KBS.md). It is implemented against the documented
// Linux TDX guest ABI but is not — and cannot be — verified on non-TEE CI/dev.

// ErrTEEUnavailable means real TEE hardware (a TDX guest device) is not present,
// so no genuine quote can be produced. The runner must fail closed.
var ErrTEEUnavailable = errors.New("compute: TEE hardware unavailable (no /dev/tdx_guest)")

const tdxGuestDevice = "/dev/tdx_guest"

// TDX guest report ioctl (Linux uapi/asm-generic): TDX_CMD_GET_REPORT0 =
// _IOWR('T', 1, struct tdx_report_req). dir=READ|WRITE(3), type='T'(0x54), nr=1,
// size=sizeof(struct tdx_report_req)=64+1024=1088 → 0xc4405401. Kept explicit so
// a node operator can cross-check against their kernel headers.
const tdxCmdGetReport0 = 0xc4405401

// tdxReportReq mirrors struct tdx_report_req { __u8 reportdata[64]; __u8 tdreport[1024]; }.
type tdxReportReq struct {
	reportData [64]byte
	tdReport   [1024]byte
}

type tdxAttester struct{}

// NewTDXAttester returns an Attester backed by the TDX guest device. On non-TEE
// hosts every Attest call fails closed with ErrTEEUnavailable.
func NewTDXAttester() Attester { return tdxAttester{} }

// Attest reads a TD report bound to the (measurement|job|output) tuple and wraps
// it as a tdx-quote-1 attestation. Fails closed when no TDX device is present.
func (tdxAttester) Attest(_ context.Context, in AttestInput) ([]byte, error) {
	if in.Measurement == "" {
		return nil, fmt.Errorf("attestation requires a measurement (algorithm image digest)")
	}
	if _, err := os.Stat(tdxGuestDevice); err != nil {
		return nil, ErrTEEUnavailable // fail closed: no hardware, no quote
	}
	report, err := readTDReport(bindingDigest(in))
	if err != nil {
		return nil, fmt.Errorf("tdx: read TD report: %w", err)
	}
	return json.Marshal(Attestation{
		Format: "tdx-quote-1", Measurement: in.Measurement, JobID: in.JobID,
		OutputSHA: in.OutputSHA, Quote: base64.StdEncoding.EncodeToString(report), Signer: "tdx",
	})
}

// Verify parses the quote envelope. For TDX, cryptographic genuineness is checked
// by the KBS/DCAP at release time, NOT in-process — so Verify reports Verified=false
// and defers, rather than overclaiming. (The buyer-facing trust is the KBS release.)
func (tdxAttester) Verify(_ context.Context, report []byte) (Attestation, error) {
	var a Attestation
	if err := json.Unmarshal(report, &a); err != nil {
		return Attestation{}, fmt.Errorf("tdx: parse attestation: %w", err)
	}
	a.Verified = false // genuineness established by DCAP/KBS, not this process
	return a, nil
}

// bindingDigest is the 64-byte REPORTDATA the TD report commits to: it binds the
// hardware quote to exactly the code (measurement), job, and output we expect.
func bindingDigest(in AttestInput) [64]byte {
	sum := sha256.Sum256([]byte(in.Measurement + "|" + in.JobID + "|" + in.OutputSHA))
	var rd [64]byte
	copy(rd[:], sum[:]) // first 32 bytes carry the binding; the rest stay zero
	return rd
}

// readTDReport performs the TDX guest ioctl. RUNS ONLY INSIDE A TDX GUEST and is
// validated there (see deploy doc); off-hardware callers never reach it because
// Attest stat-gates on the device first.
func readTDReport(reportData [64]byte) ([]byte, error) {
	f, err := os.OpenFile(tdxGuestDevice, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", tdxGuestDevice, err)
	}
	defer f.Close()
	req := tdxReportReq{reportData: reportData}
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), uintptr(tdxCmdGetReport0),
		uintptr(unsafe.Pointer(&req))); errno != 0 {
		return nil, fmt.Errorf("ioctl TDX_CMD_GET_REPORT0: %w", errno)
	}
	out := make([]byte, len(req.tdReport))
	copy(out, req.tdReport[:])
	return out, nil
}
