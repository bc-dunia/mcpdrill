import { memo } from 'react';

interface HelpTooltipProps {
  text: string;
}

export const HelpTooltip = memo(function HelpTooltip({ text }: HelpTooltipProps) {
  const tooltipId = `help-${Math.random().toString(36).substring(2, 9)}`;
  return (
    <button type="button" className="help-tooltip" aria-describedby={tooltipId} aria-label="Help">
      ?
      <span id={tooltipId} className="help-tooltip-content" role="tooltip">
        {text}
      </span>
    </button>
  );
});
