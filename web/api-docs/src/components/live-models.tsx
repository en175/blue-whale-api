'use client';

import { STATIC_MODELS, L, type DocModel } from './live-models.data';

function ModelCard({ m }: { m: DocModel }) {
  return (
    <div className="rounded-lg border p-4">
      <div className="flex items-center gap-2">
        <span className="font-mono font-semibold">{m.model_name}</span>
        {m.tag ? (
          <span className="bg-primary/10 text-primary rounded-full px-2 py-0.5 text-xs font-medium">
            {m.tag}
          </span>
        ) : null}
      </div>
      {m.description ? (
        <p className="text-muted-foreground mt-1 text-sm">{m.description}</p>
      ) : null}
    </div>
  );
}

function Section({ title, models }: { title: string; models: DocModel[] }) {
  if (models.length === 0) {
    return null;
  }

  return (
    <section>
      <h2 className="mb-4 text-lg font-semibold">{title}</h2>
      <div className="grid gap-4 sm:grid-cols-2">
        {models.map((m) => (
          <ModelCard key={m.model_name} m={m} />
        ))}
      </div>
    </section>
  );
}

function byCategory(
  category: DocModel['category'],
  region?: DocModel['region']
) {
  return STATIC_MODELS.filter(
    (m) =>
      m.category === category && (region === undefined || m.region === region)
  ).sort((a, b) => a.sort_order - b.sort_order);
}

export function LiveModels() {
  if (STATIC_MODELS.length === 0) {
    return <p className="text-sm">{L.empty}</p>;
  }

  return (
    <div className="space-y-8">
      <Section title={L.section.text} models={byCategory('text')} />
      <Section title={L.section.image} models={byCategory('image')} />
      <Section
        title={L.section.videoDomestic}
        models={byCategory('video', 'domestic')}
      />
      <Section
        title={L.section.videoOverseas}
        models={byCategory('video', 'overseas')}
      />
    </div>
  );
}
