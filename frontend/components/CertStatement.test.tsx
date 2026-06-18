import { describe, it, expect, beforeEach } from "vitest";
import { render } from "@testing-library/react";
import { LocaleProvider } from "@/lib/i18n";
import { CertStatement } from "./CertStatement";

function renderWithLang(lang: "zh" | "en") {
  window.localStorage.setItem("vo_lang", lang);
  return render(
    <LocaleProvider>
      <CertStatement zh="ZH-STATEMENT" en="EN-STATEMENT" />
    </LocaleProvider>,
  );
}

describe("CertStatement", () => {
  beforeEach(() => window.localStorage.clear());

  it("leads with the English statement when the UI language is English", () => {
    const { container } = renderWithLang("en");
    const ps = container.querySelectorAll("p");
    expect(ps[0].textContent).toBe("EN-STATEMENT");
    expect(ps[1].textContent).toBe("ZH-STATEMENT");
  });

  it("leads with the Chinese statement when the UI language is Chinese", () => {
    const { container } = renderWithLang("zh");
    const ps = container.querySelectorAll("p");
    expect(ps[0].textContent).toBe("ZH-STATEMENT");
    expect(ps[1].textContent).toBe("EN-STATEMENT");
  });
});
