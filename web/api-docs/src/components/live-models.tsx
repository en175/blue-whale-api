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

export function LiveModels() {
  if (STATIC_MODELS.length === 0) {
    return <p className="text-sm">{L.empty}</p>;
  }

  const domestic = STATIC_MODELS.filter((m) => m.region === 'domestic').sort(
    (a, b) => a.sort_order - b.sort_order
  );
  const overseas = STATIC_MODELS.filter((m) => m.region === 'overseas').sort(
    (a, b) => a.sort_order - b.sort_order
  );

  return (
    <div className="space-y-8">
      {domestic.length > 0 ? (
        <section>
          <h2 className="mb-4 text-lg font-semibold">{L.section.domestic}</h2>
          <div className="grid gap-4 sm:grid-cols-2">
            {domestic.map((m) => (
              <ModelCard key={m.model_name} m={m} />
            ))}
          </div>
        </section>
      ) : null}
      {overseas.length > 0 ? (
        <section>
          <h2 className="mb-4 text-lg font-semibold">{L.section.overseas}</h2>
          <div className="grid gap-4 sm:grid-cols-2">
            {overseas.map((m) => (
              <ModelCard key={m.model_name} m={m} />
            ))}
          </div>
        </section>
      ) : null}
    </div>
  );
}
