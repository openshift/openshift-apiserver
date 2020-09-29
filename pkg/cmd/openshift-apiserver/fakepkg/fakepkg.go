package fakepkg

import "k8s.io/klog/v2"

func CallMe() {
	for i := 0; i < 10 ; i++ {
		klog.Info("IF YOU SEE THIS LOG THEN IT MEANS KLOG V2 IS ALIVE (%d)", i)
	}
}
