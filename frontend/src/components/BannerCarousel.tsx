import totalBanner from '../assets/banners/totalbanner.png';

export function BannerCarousel() {
  return (
    <div className="banner-carousel">
      <div className="banner-viewport" aria-hidden>
        <div className="banner-slide">
          <img src={totalBanner} alt="" draggable={false} />
        </div>
      </div>
      <div className="banner-shade" aria-hidden />
    </div>
  );
}
