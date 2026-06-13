import { describe, expect, it } from "vitest";
import { render } from "@testing-library/react";
import { MiniChart } from "./MiniChart";

describe("MiniChart", () => {
  it("renders an em-dash placeholder when there are no points", () => {
    const { container } = render(<MiniChart points={[]} />);
    expect(container.textContent).toContain("—");
    expect(container.querySelector("svg")).toBeNull();
  });

  it("renders an svg polyline when given points", () => {
    const { container } = render(
      <MiniChart points={[{ date: "2026-06-01", value: 1 }, { date: "2026-06-02", value: 5 }]} />,
    );
    expect(container.querySelector("svg")).not.toBeNull();
  });
});
