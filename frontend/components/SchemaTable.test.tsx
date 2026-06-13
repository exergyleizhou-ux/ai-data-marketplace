import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import type { QualityCheck } from "@/lib/api";
import { SchemaTable } from "./SchemaTable";

describe("SchemaTable", () => {
  it("shows a non-tabular notice when there is no applicable schema check", () => {
    render(<SchemaTable checks={[]} />);
    expect(screen.getByText(/Not tabular/)).toBeInTheDocument();
  });

  it("renders row/column counts for an applicable schema check", () => {
    const checks: QualityCheck[] = [
      { type: "schema", result: "pass", report: { applicable: true, row_count: 42, column_count: 3, columns: [], alerts: [] } },
    ];
    render(<SchemaTable checks={checks} />);
    expect(screen.getByText(/42/)).toBeInTheDocument();
  });
});
