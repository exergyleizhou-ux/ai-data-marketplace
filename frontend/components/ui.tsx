"use client";

import Link from "next/link";
import { type ButtonHTMLAttributes, type InputHTMLAttributes, type ReactNode, type SelectHTMLAttributes, type TextareaHTMLAttributes } from "react";

export function Button({
  variant = "primary",
  className = "",
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: "primary" | "secondary" | "danger" | "ghost" }) {
  const base =
    "inline-flex items-center justify-center rounded-md px-4 py-2 text-sm font-medium transition disabled:opacity-50 disabled:cursor-not-allowed";
  const variants = {
    primary: "bg-neutral-900 text-white hover:bg-neutral-700",
    secondary: "border border-neutral-300 bg-white text-neutral-800 hover:bg-neutral-50",
    danger: "bg-red-600 text-white hover:bg-red-500",
    ghost: "text-neutral-600 hover:bg-neutral-100",
  };
  return <button className={`${base} ${variants[variant]} ${className}`} {...props} />;
}

export function Input({ className = "", ...props }: InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      className={`w-full rounded-md border border-neutral-300 px-3 py-2 text-sm outline-none focus:border-neutral-900 ${className}`}
      {...props}
    />
  );
}

export function Textarea({ className = "", ...props }: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return (
    <textarea
      className={`w-full rounded-md border border-neutral-300 px-3 py-2 text-sm outline-none focus:border-neutral-900 ${className}`}
      {...props}
    />
  );
}

export function Select({ className = "", ...props }: SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <select
      className={`w-full rounded-md border border-neutral-300 bg-white px-3 py-2 text-sm outline-none focus:border-neutral-900 ${className}`}
      {...props}
    />
  );
}

export function Field({ label, children, hint }: { label: string; children: ReactNode; hint?: string }) {
  return (
    <label className="block space-y-1">
      <span className="text-sm font-medium text-neutral-700">{label}</span>
      {children}
      {hint && <span className="block text-xs text-neutral-400">{hint}</span>}
    </label>
  );
}

export function Card({ children, className = "" }: { children: ReactNode; className?: string }) {
  return <div className={`rounded-xl border border-neutral-200 bg-white p-5 shadow-sm ${className}`}>{children}</div>;
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

export function Spinner({ label = "加载中…" }: { label?: string }) {
  return <div className="py-10 text-center text-sm text-neutral-400">{label}</div>;
}

export function Empty({ children }: { children: ReactNode }) {
  return <div className="rounded-lg border border-dashed border-neutral-300 py-12 text-center text-sm text-neutral-400">{children}</div>;
}

export function LinkButton({ href, children }: { href: string; children: ReactNode }) {
  return (
    <Link href={href} className="text-sm font-medium text-neutral-900 underline-offset-2 hover:underline">
      {children}
    </Link>
  );
}
