import { memo } from 'react';

interface Props {
  term: string;
  definition: string;
  children: React.ReactNode;
}

export const GlossaryTerm = memo(function GlossaryTerm({ term, definition, children }: Props) {
  const tooltipId = `glossary-${term.replace(/\s+/g, '-').toLowerCase()}`;
  
  return (
    <span className="glossary-term" aria-describedby={tooltipId}>
      {children}
      <span className="glossary-icon" aria-hidden="true">i</span>
      <span id={tooltipId} className="glossary-tooltip" role="tooltip">
        <strong>{term}</strong>: {definition}
      </span>
    </span>
  );
});
