import { describe, expect, it, afterEach } from "vitest";
import { render, screen } from "@testing-library/react";
import type { QualityCheck } from "@/lib/api";
import { LocaleProvider } from "@/lib/i18n";
import { QualityReport, QualityBadge } from "./QualityReport";

afterEach(() => window.localStorage.clear());

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

  it("renders the quality badge in the active UI language", () => {
    window.localStorage.setItem("vo_lang", "en");
    render(
      <LocaleProvider>
        <QualityBadge verified />
      </LocaleProvider>,
    );
    expect(screen.getByText(/Quality verified/)).toBeInTheDocument();
    expect(screen.queryByText(/质检通过/)).toBeNull();
  });
});
