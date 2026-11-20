// Scrolling strip of university logos (Wikimedia Commons SVGs), shown in one
// plain grey so no single brand stands out.
const FILES = import.meta.glob('./assets/logos/*.svg', { eager: true, query: '?url', import: 'default' });
const url = (slug) => FILES[`./assets/logos/${slug}.svg`];

// Countries alternate so the strip doesn't bunch by country.
const LOGOS = [
  { slug: 'polimi', name: 'Politecnico di Milano' },
  { slug: 'tum', name: 'Technical University of Munich' },
  { slug: 'gatech', name: 'Georgia Institute of Technology' },
  { slug: 'bocconi', name: 'Bocconi University' },
  { slug: 'rwth', name: 'RWTH Aachen University' },
  { slug: 'umich', name: 'University of Michigan' },
  { slug: 'unibo', name: 'University of Bologna' },
  { slug: 'purdue', name: 'Purdue University' },
  { slug: 'polito', name: 'Politecnico di Torino' },
  { slug: 'utexas', name: 'The University of Texas at Austin' },
  { slug: 'unitn', name: 'University of Trento' },
  { slug: 'uiuc', name: 'University of Illinois Urbana-Champaign' },
  { slug: 'neu', name: 'Northeastern University' },
  { slug: 'asu', name: 'Arizona State University' },
].filter((l) => url(l.slug)); // official names, never translated

export default function LogoMarquee() {
  // Two identical halves: the track slides by -50% and loops seamlessly.
  const half = (copy) => LOGOS.map(({ slug, name }) => (
    <img key={`${copy}-${slug}`} className={`uni-logo l-${slug}`} src={url(slug)} alt={copy ? '' : name} aria-hidden={copy ? 'true' : undefined} loading="lazy" draggable="false" />
  ));
  return (
    <div className="marquee" role="list" aria-label="Universities">
      <div className="marquee-track">{half(0)}{half(1)}</div>
    </div>
  );
}
