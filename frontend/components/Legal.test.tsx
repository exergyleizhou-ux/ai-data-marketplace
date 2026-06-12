import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { LegalHeader, LegalFooterNav } from "./Legal";

describe("LegalHeader", () => {
  it("renders the bilingual title", () => {
    render(<LegalHeader titleZh="用户服务协议" titleEn="Terms of Service" />);
    expect(screen.getByRole("heading", { name: /用户服务协议/ })).toBeInTheDocument();
    expect(screen.getByText(/Terms of Service/)).toBeInTheDocument();
  });
});

describe("LegalFooterNav", () => {
  it("links to privacy but not back to the current terms page", () => {
    render(<LegalFooterNav current="terms" />);
    expect(screen.getByRole("link", { name: /Privacy Policy/ })).toHaveAttribute("href", "/privacy");
    expect(screen.queryByRole("link", { name: /Terms of Service/ })).toBeNull();
  });

  it("links to terms when on the privacy page", () => {
    render(<LegalFooterNav current="privacy" />);
    expect(screen.getByRole("link", { name: /Terms of Service/ })).toHaveAttribute("href", "/terms");
    expect(screen.queryByRole("link", { name: /Privacy Policy/ })).toBeNull();
  });
});
