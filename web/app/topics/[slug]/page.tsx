import Link from "next/link";
import type { Metadata } from "next";
import { notFound } from "next/navigation";

import { AdSlot } from "@/components/ad-slot";
import { SiteChrome } from "@/components/site-chrome";
import { absoluteUrl, siteName } from "@/lib/site";
import { findTopicPage, topicPages } from "@/lib/topic-pages";

// 紹介ページの文章はコードの中にあるため、ビルド時に静的に生成する。用意していない
// slugは404にする。
export const dynamicParams = false;

export function generateStaticParams() {
  return topicPages.map((page) => ({ slug: page.slug }));
}

export async function generateMetadata({ params }: PageProps<"/topics/[slug]">): Promise<Metadata> {
  const { slug } = await params;
  const page = findTopicPage(slug);
  if (page === undefined) {
    return {};
  }

  const path = `/topics/${page.slug}`;
  return {
    title: page.title,
    description: page.description,
    alternates: { canonical: path },
    openGraph: {
      title: `${page.title} | ${siteName}`,
      description: page.description,
      url: path,
    },
  };
}

const steps = [
  "このページのボタンから、トピックを選ぶ画面に進みます。",
  "2 人で話すか、3 人で話すかを選びます。",
  "同じトピックを選んだ人が集まると、チャットが始まります。",
];

const stepsHeading = "チャットの始め方";
const ideasHeading = "こんな話ができます";
const otherTopicsHeading = "ほかのトピック";
const purpose = `${siteName}は、話題について話すための匿名チャットです。出会いや交際を目的とした利用、連絡先の交換はできません。`;

export default async function TopicPage({ params }: PageProps<"/topics/[slug]">) {
  const { slug } = await params;
  const page = findTopicPage(slug);
  if (page === undefined) {
    notFound();
  }

  const startHref = `/topics?topic=${encodeURIComponent(page.slug)}`;
  const others = topicPages.filter((other) => other.slug !== page.slug);

  const jsonLd = {
    "@context": "https://schema.org",
    "@type": "BreadcrumbList",
    itemListElement: [
      { "@type": "ListItem", position: 1, name: siteName, item: absoluteUrl("/") },
      { "@type": "ListItem", position: 2, name: "トピックを選ぶ", item: absoluteUrl("/topics") },
      {
        "@type": "ListItem",
        position: 3,
        name: page.topicName,
        item: absoluteUrl(`/topics/${page.slug}`),
      },
    ],
  };

  return (
    <SiteChrome>
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: JSON.stringify(jsonLd).replace(/</g, "\\u003c") }}
      />

      <article className="mx-auto w-full max-w-3xl px-5 py-16">
        <p className="text-brand-ink text-sm font-bold tracking-wide">{page.topicName}</p>
        <h1 className="mt-4 text-3xl leading-tight font-bold tracking-tight sm:text-4xl">
          {page.heading}
        </h1>
        <p className="text-muted mt-6 text-lg leading-8">{page.lead}</p>
        <Link
          href={startHref}
          className="bg-brand text-brand-contrast hover:bg-brand-hover shadow-brand mt-10 inline-flex h-12 items-center justify-center rounded-full px-8 text-base font-bold transition-colors"
        >
          {page.cta}
        </Link>

        <section className="mt-16">
          <h2 className="text-2xl font-bold tracking-tight">{ideasHeading}</h2>
          <ul className="mt-6 grid gap-3 sm:grid-cols-2">
            {page.ideas.map((idea) => (
              <li key={idea} className="border-line rounded-2xl border p-5 leading-7">
                {idea}
              </li>
            ))}
          </ul>
          <p className="text-muted mt-6 leading-7">{page.note}</p>
        </section>

        <section className="mt-16">
          <h2 className="text-2xl font-bold tracking-tight">{stepsHeading}</h2>
          <ol className="mt-6 grid gap-4">
            {steps.map((step, index) => (
              <li key={step} className="flex gap-4">
                <span className="bg-brand-soft text-brand-ink flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-sm font-bold">
                  {index + 1}
                </span>
                <p className="leading-8">{step}</p>
              </li>
            ))}
          </ol>
          <Link
            href={startHref}
            className="bg-brand text-brand-contrast hover:bg-brand-hover shadow-brand mt-10 inline-flex h-12 items-center justify-center rounded-full px-8 text-base font-bold transition-colors"
          >
            {page.cta}
          </Link>
        </section>

        <AdSlot className="mt-16" />

        <section className="mt-16">
          <h2 className="text-2xl font-bold tracking-tight">{otherTopicsHeading}</h2>
          <ul className="mt-6 flex flex-wrap gap-3">
            {others.map((other) => (
              <li key={other.slug}>
                <Link
                  href={`/topics/${other.slug}`}
                  className="border-line hover:bg-surface inline-flex h-11 items-center justify-center rounded-full border px-5 text-sm font-medium transition-colors"
                >
                  {other.topicName}
                </Link>
              </li>
            ))}
          </ul>
        </section>

        <p className="border-line text-muted mt-16 rounded-2xl border border-dashed p-6 leading-7">
          {purpose}
        </p>
      </article>
    </SiteChrome>
  );
}
