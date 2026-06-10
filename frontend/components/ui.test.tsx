import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Button, Alert, Field, Input, Badge } from "./ui";

describe("ui primitives", () => {
  it("Button exposes a button role and fires onClick", async () => {
    const onClick = vi.fn();
    render(<Button onClick={onClick}>提交</Button>);
    const btn = screen.getByRole("button", { name: "提交" });
    await userEvent.click(btn);
    expect(onClick).toHaveBeenCalledOnce();
  });

  it("Button respects disabled", async () => {
    const onClick = vi.fn();
    render(<Button disabled onClick={onClick}>提交</Button>);
    const btn = screen.getByRole("button", { name: "提交" });
    expect(btn).toBeDisabled();
    await userEvent.click(btn);
    expect(onClick).not.toHaveBeenCalled();
  });

  it("Field associates its label with the nested input", () => {
    render(<Field label="账号"><Input defaultValue="" /></Field>);
    expect(screen.getByLabelText("账号")).toBeInTheDocument();
  });

  it("Alert renders its message", () => {
    render(<Alert kind="error">出错了</Alert>);
    expect(screen.getByText("出错了")).toBeInTheDocument();
  });

  it("Badge renders its status text", () => {
    render(<Badge>settled</Badge>);
    expect(screen.getByText("settled")).toBeInTheDocument();
  });
});
