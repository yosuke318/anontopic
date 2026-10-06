"use client";

import { useSyncExternalStore } from "react";

import { termsVersion } from "@/lib/terms";

const storageKey = "anontopic:terms-accepted";

const listeners = new Set<() => void>();

function readAcceptedVersion(): string | null {
  try {
    return window.localStorage.getItem(storageKey);
  } catch {
    return null;
  }
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  window.addEventListener("storage", listener);
  return () => {
    listeners.delete(listener);
    window.removeEventListener("storage", listener);
  };
}

// 保存できないブラウザ（ストレージを拒否している場合など）では、毎回同意を求める。
export function acceptTerms(): void {
  try {
    window.localStorage.setItem(storageKey, termsVersion);
  } catch {
    return;
  }
  listeners.forEach((listener) => listener());
}

// useTermsAcceptedは今の版に同意済みかを返す。サーバー側の描画では未同意として扱う。
export function useTermsAccepted(): boolean {
  return useSyncExternalStore(
    subscribe,
    () => readAcceptedVersion() === termsVersion,
    () => false,
  );
}
