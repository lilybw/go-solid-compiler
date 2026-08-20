import { template as _$template } from "solid-js/web";
import { insert as _$insert } from "solid-js/web";
import { Show as _$Show } from "solid-js/web";
import { createComponent as _$createComponent } from "solid-js/web";
import { For as _$For } from "solid-js/web";
var _tmpl$ = /*#__PURE__*/_$template(`<p>`),
  _tmpl$2 = /*#__PURE__*/_$template(`<div>`),
  _tmpl$3 = /*#__PURE__*/_$template(`<li>`),
  _tmpl$4 = /*#__PURE__*/_$template(`<span>loading`);
export const ControlFlow = () => (() => {
  var _el$ = _tmpl$2();
  _$insert(_el$, _$createComponent(_$For, {
    get each() {
      return items();
    },
    children: item => (() => {
      var _el$3 = _tmpl$3();
      _$insert(_el$3, () => item.name);
      return _el$3;
    })()
  }), null);
  _$insert(_el$, _$createComponent(_$Show, {
    get when() {
      return ready();
    },
    get fallback() {
      return _tmpl$4();
    },
    get children() {
      var _el$2 = _tmpl$();
      _$insert(_el$2, body);
      return _el$2;
    }
  }), null);
  return _el$;
})();
