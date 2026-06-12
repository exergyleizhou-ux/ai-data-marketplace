import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { LocaleProvider } from "@/lib/i18n";
import { BRAND } from "@/lib/brand";
import { SiteFooter } from "./SiteFooter";

describe("SiteFooter", () => {
  it("renders the brand name and legal links", () => {
    render(
      <LocaleProvider>
        <SiteFooter />
      </LocaleProvider>,
    );
    expect(screen.getByText(BRAND.name)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /服务协议|Terms/i })).toHaveAttribute("href", "/terms");
    expect(screen.getByRole("link", { name: /隐私|Privacy/i })).toHaveAttribute("href", "/privacy");
  });
});
