export async function exportChartAsPng(
  chartContainer: HTMLDivElement | null,
  filename: string
): Promise<void> {
  if (!chartContainer) return;

  const svg = chartContainer.querySelector('.recharts-wrapper svg') 
    ?? chartContainer.querySelector('svg.recharts-surface')
    ?? chartContainer.querySelector('svg');
  if (!svg) return;

  try {
    const svgClone = svg.cloneNode(true) as SVGElement;
    const svgRect = svg.getBoundingClientRect();
    svgClone.setAttribute('width', String(svgRect.width));
    svgClone.setAttribute('height', String(svgRect.height));
    
    const svgData = new XMLSerializer().serializeToString(svgClone);
    const svgBase64 = btoa(unescape(encodeURIComponent(svgData)));
    const svgDataUrl = `data:image/svg+xml;base64,${svgBase64}`;

    const canvas = document.createElement('canvas');
    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    const img = new Image();
    
    await new Promise<void>((resolve, reject) => {
      img.onload = () => {
        const scale = 2;
        canvas.width = img.width * scale;
        canvas.height = img.height * scale;
        
        ctx.fillStyle = '#ffffff';
        ctx.fillRect(0, 0, canvas.width, canvas.height);
        ctx.scale(scale, scale);
        ctx.drawImage(img, 0, 0);
        
        resolve();
      };
      
      img.onerror = () => reject(new Error('Failed to load SVG'));
      img.src = svgDataUrl;
    });

    const pngDataUrl = canvas.toDataURL('image/png', 1.0);
    const finalFilename = filename.endsWith('.png') ? filename : `${filename}.png`;
    
    const downloadLink = document.createElement('a');
    downloadLink.href = pngDataUrl;
    downloadLink.download = finalFilename;
    
    // Safari requires element in DOM for download attribute to work
    document.body.appendChild(downloadLink);
    downloadLink.click();
    setTimeout(() => document.body.removeChild(downloadLink), 100);
    
  } catch (error) {
    console.error('Chart export failed:', error);
  }
}
