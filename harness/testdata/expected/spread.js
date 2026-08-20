import { template as _$template } from "solid-js/web";
import { insert as _$insert } from "solid-js/web";
import { createComponent as _$createComponent } from "solid-js/web";
import { mergeProps as _$mergeProps } from "solid-js/web";
var _tmpl$ = /*#__PURE__*/_$template(`<div>`);
export const Spread = () => (() => {
  var _el$ = _tmpl$();
  _$insert(_el$, _$createComponent(Card, _$mergeProps(rest, {
    get id() {
      return id();
    }
  })));
  return _el$;
})();
