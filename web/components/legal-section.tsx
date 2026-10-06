import type { ReactNode } from "react";

// LegalSectionは規約やポリシーの1つの条を描く。見出しへのリンクで直接開けるよう id を付ける。
export function LegalSection({
  id,
  title,
  children,
}: {
  id: string;
  title: string;
  children: ReactNode;
}) {
  return (
    <section id={id} className="mt-12 scroll-mt-20">
      <h2 className="text-xl font-bold tracking-tight">{title}</h2>
      <div className="text-muted mt-4 space-y-4 leading-7">{children}</div>
    </section>
  );
}

export function LegalList({ items }: { items: string[] }) {
  return (
    <ul className="list-disc space-y-2 pl-5">
      {items.map((item) => (
        <li key={item}>{item}</li>
      ))}
    </ul>
  );
}
