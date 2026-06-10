import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { LocaleProvider, LangToggle, useT } from "./i18n";

// A tiny consumer that surfaces the current language and a t(zh,en) result.
function Probe() {
  const { lang, t } = useT();
  return (
    <div>
      <span data-testid="lang">{lang}</span>
      <span data-testid="word">{t("账号", "Account")}</span>
    </div>
  );
}

function renderWithLocale() {
  return render(
    <LocaleProvider>
      <LangToggle />
      <Probe />
    </LocaleProvider>,
  );
}

describe("i18n", () => {
  it("honors a persisted Chinese preference and resolves t() to zh", async () => {
    localStorage.setItem("vo_lang", "zh");
    renderWithLocale();
    expect(await screen.findByTestId("lang")).toHaveTextContent("zh");
    expect(screen.getByTestId("word")).toHaveTextContent("账号");
  });

  it("honors a persisted English preference and resolves t() to en", async () => {
    localStorage.setItem("vo_lang", "en");
    renderWithLocale();
    expect(await screen.findByTestId("lang")).toHaveTextContent("en");
    expect(screen.getByTestId("word")).toHaveTextContent("Account");
  });

  it("toggles language and persists the choice", async () => {
    localStorage.setItem("vo_lang", "zh");
    renderWithLocale();
    expect(await screen.findByTestId("lang")).toHaveTextContent("zh");
    // In zh the toggle offers EN; clicking flips to en.
    await userEvent.click(screen.getByRole("button", { name: /Switch to English/i }));
    expect(screen.getByTestId("lang")).toHaveTextContent("en");
    expect(localStorage.getItem("vo_lang")).toBe("en");
  });
});
