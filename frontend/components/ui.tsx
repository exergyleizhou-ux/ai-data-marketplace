"use client";

import Link from "next/link";
import { type ButtonHTMLAttributes, type InputHTMLAttributes, type ReactNode, type SelectHTMLAttributes, type TextareaHTMLAttributes } from "react";

export function Button({
  variant = "primary",
  className = "",
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: "primary" | "secondary" | "danger" | "ghost" }) {
  const base =
    "inline-flex items-center justify-center rounded-full px-5 py-2 text-sm font-medium transition disabled:opacity-50 disabled:cursor-not-allowed focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ink focus-visible:ring-offset-2";
  const variants = {
    primary: "bg-ink text-paper hover:bg-ink/85",
    secondary: "border border-rule bg-white text-ink hover:bg-paper",
    danger: "bg-red-700 text-paper hover:bg-red-600",
    ghost: "text-ink/70 hover:bg-paper",
  };
  return <button className={`${base} ${variants[variant]} ${className}`} {...props} />;
}

export function Input({ className = "", ...props }: InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      className={`w-full rounded-lg border border-rule bg-white px-3 py-2 text-sm text-ink outline-none transition focus:border-ink focus:ring-2 focus:ring-ink/5 ${className}`}
      {...props}
    />
  );
}

export function Textarea({ className = "", ...props }: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return (
    <textarea
      className={`w-full rounded-lg border border-rule bg-white px-3 py-2 text-sm text-ink outline-none transition focus:border-ink focus:ring-2 focus:ring-ink/5 ${className}`}
      {...props}
    />
  );
}

export function Select({ className = "", ...props }: SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <select
      className={`w-full rounded-lg border border-rule bg-white px-3 py-2 text-sm text-ink outline-none transition focus:border-ink focus:ring-2 focus:ring-ink/5 ${className}`}
      {...props}
    />
  );
}

export function Field({ label, children, hint }: { label: string; children: ReactNode; hint?: string }) {
  return (
    <label className="block space-y-1.5">
      <span className="text-sm font-medium text-ink">{label}</span>
      {children}
      {hint && <span className="block text-xs text-muted">{hint}</span>}
    </label>
  );
}

export function Card({ children, className = "" }: { children: ReactNode; className?: string }) {
  return <div className={`rounded-2xl border border-rule bg-white p-6 ${className}`}>{children}</div>;
}

const STATUS_COLORS: Record<string, string> = {
  // datasets
  draft: "bg-neutral-100 text-neutral-600",
  uploading: "bg-blue-50 text-blue-600",
  checking: "bg-amber-50 text-amber-700",
  reviewing: "bg-purple-50 text-purple-700",
  published: "bg-green-50 text-green-700",
  rejected: "bg-red-50 text-red-700",
  delisted: "bg-neutral-200 text-neutral-500",
  // orders
  created: "bg-neutral-100 text-neutral-600",
  paid: "bg-blue-50 text-blue-700",
  delivered: "bg-indigo-50 text-indigo-700",
  confirmed: "bg-teal-50 text-teal-700",
  settled: "bg-green-50 text-green-700",
  disputed: "bg-orange-50 text-orange-700",
  refunded: "bg-red-50 text-red-700",
  cancelled: "bg-neutral-200 text-neutral-500",
  // kyc
  none: "bg-neutral-100 text-neutral-600",
  pending: "bg-amber-50 text-amber-700",
  verified: "bg-green-50 text-green-700",
  // withdrawals (rejected already mapped under datasets)
  approved: "bg-blue-50 text-blue-700",
  completed: "bg-green-50 text-green-700",
};

export function Badge({ children }: { children: string }) {
  const cls = STATUS_COLORS[children] ?? "bg-neutral-100 text-neutral-600";
  return <span className={`inline-block rounded-full px-2.5 py-0.5 text-xs font-medium ${cls}`}>{children}</span>;
}

export function Alert({ kind = "error", children }: { kind?: "error" | "success" | "info"; children: ReactNode }) {
  const cls = {
    error: "border-red-200 bg-red-50 text-red-700",
    success: "border-green-200 bg-green-50 text-green-700",
    info: "border-blue-200 bg-blue-50 text-blue-700",
  }[kind];
  return <div className={`rounded-md border px-3 py-2 text-sm ${cls}`}>{children}</div>;
}

// Default label intentionally stays bilingual so callers that omit `label`
// (e.g. <Spinner />) still respect the active language. Pass an explicit string
// when you want a fixed-language label.
function defaultSpinnerLabel() {
  if (typeof navigator !== "undefined" && navigator.language && navigator.language.toLowerCase().startsWith("zh")) {
    return "加载中…";
  }
  return "Loading…";
}
export function Spinner({ label }: { label?: string }) {
  const text = label ?? defaultSpinnerLabel();
  // role=status + aria-busy so screen readers announce the loading state.
  // Callers pass a localized label (Spinner stays provider-agnostic).
  return (
    <div role="status" aria-busy="true" aria-label={text} className="py-10 text-center text-sm text-muted">
      {text}
    </div>
  );
}

export function Empty({ children }: { children: ReactNode }) {
  return <div className="rounded-2xl border border-dashed border-rule py-12 text-center text-sm text-muted">{children}</div>;
}

export function LinkButton({ href, children }: { href: string; children: ReactNode }) {
  return (
    <Link href={href} className="text-sm font-medium text-neutral-900 underline-offset-2 hover:underline">
      {children}
    </Link>
  );
}
