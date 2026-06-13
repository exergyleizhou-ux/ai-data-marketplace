import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import type { Datasheet } from "@/lib/api";
import { DatasheetView } from "./Datasheet";

describe("DatasheetView", () => {
  it("shows an empty notice when no fields are filled", () => {
    render(<DatasheetView ds={{}} />);
    expect(screen.getByText(/No datasheet provided yet/)).toBeInTheDocument();
  });

  it("renders filled fields and languages", () => {
    const ds: Datasheet = {
      intended_uses: "Training classifiers on Chinese legal text",
      languages: ["zh", "en"],
    };
    render(<DatasheetView ds={ds} />);
    expect(screen.getByText(/Training classifiers/)).toBeInTheDocument();
    expect(screen.getByText("zh")).toBeInTheDocument();
  });
});
