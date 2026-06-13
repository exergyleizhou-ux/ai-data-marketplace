import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import type { QualityCheck } from "@/lib/api";
import { QualityReport } from "./QualityReport";

describe("QualityReport", () => {
  it("shows a not-yet-available notice when there are no checks", () => {
    render(<QualityReport checks={[]} />);
    expect(screen.getByText(/Quality screening not yet available/)).toBeInTheDocument();
  });

  it("renders the signals-not-verdicts disclaimer when checks exist", () => {
    const checks: QualityCheck[] = [
      { type: "format", result: "pass", report: {} },
      { type: "stats", result: "warn", report: {} },
    ];
    render(<QualityReport checks={checks} />);
    expect(screen.getByText(/statistical signals, not verdicts/)).toBeInTheDocument();
  });
});
